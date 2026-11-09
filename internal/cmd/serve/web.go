package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/carlmjohnson/versioninfo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/1995parham-me/didban/internal/config"
	"github.com/1995parham-me/didban/internal/infra/station"
	"github.com/1995parham-me/didban/internal/infra/web"
	"github.com/1995parham-me/didban/internal/sensor"
)

const (
	// historySize × historyStep is how far back the page's charts reach.
	historySize = 24 * 60
	historyStep = time.Minute

	// maxBeeps caps one POST /api/beep, so the page cannot hold the buzzer.
	maxBeeps = 5
	beepOn   = 150 * time.Millisecond
)

// latest is the newest reading, shared by the poller and the HTTP handlers.
type latest struct {
	mu      sync.Mutex
	reading *sensor.Reading
}

func (l *latest) set(r sensor.Reading) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.reading = &r
}

func (l *latest) get() *sensor.Reading {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.reading
}

// server is everything the HTTP handlers read.
type server struct {
	cfg     config.Config
	reg     *prometheus.Registry
	st      *station.Station
	msg     *message
	hist    *sensor.History
	now     *latest
	started time.Time
	host    string
	beeping sync.Mutex
}

// status is the GET /api/status body.
type status struct {
	Host            string          `json:"host"`
	Version         string          `json:"version"`
	Started         time.Time       `json:"started"`
	Now             time.Time       `json:"now"`
	Timezone        string          `json:"timezone"`
	PollSeconds     float64         `json:"poll_seconds"`
	Sensors         map[string]bool `json:"sensors"`
	Reading         *sensor.Reading `json:"reading,omitempty"`
	RTCDriftSeconds *float64        `json:"rtc_drift_seconds,omitempty"`
}

// beepRequest is the POST /api/beep body.
type beepRequest struct {
	Count int `json:"count"`
}

func newServer(cfg config.Config, reg *prometheus.Registry, st *station.Station, msg *message) *server {
	host, err := os.Hostname()
	if err != nil {
		host = "didban"
	}

	return &server{
		cfg:     cfg,
		reg:     reg,
		st:      st,
		msg:     msg,
		hist:    sensor.NewHistory(historySize, historyStep),
		now:     &latest{},
		started: time.Now(),
		host:    host,
	}
}

// observe records a fresh reading for the page.
func (s *server) observe(r sensor.Reading) {
	s.now.set(r)
	s.hist.Add(r)
}

// handler builds the HTTP mux: the status page and its JSON API, /metrics for
// Prometheus, and POST /lcd for messages.
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()

	files := http.FileServerFS(web.Static())
	mux.Handle("GET /{$}", noCache(files))
	mux.Handle("GET /static/", noCache(http.StripPrefix("/static", files)))

	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/history", s.history)
	mux.HandleFunc("POST /api/beep", s.beep)
	mux.HandleFunc("POST /lcd", s.lcd)

	mux.Handle("GET /metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))

	return mux
}

// noCache makes the browser revalidate the embedded files, so a new binary's
// page shows up on the next load.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

func (s *server) status(w http.ResponseWriter, _ *http.Request) {
	body := status{
		Host:        s.host,
		Version:     versioninfo.Short(),
		Started:     s.started,
		Now:         time.Now(),
		Timezone:    s.cfg.DS3231.Timezone,
		PollSeconds: s.cfg.Poll.Interval.Seconds(),
		Sensors:     s.st.Sensors(),
		Reading:     s.now.get(),
	}

	if r := body.Reading; r != nil && r.RTCTime != nil {
		drift := r.RTCTime.Sub(r.At).Seconds()
		body.RTCDriftSeconds = &drift
	}

	writeJSON(w, body)
}

func (s *server) history(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.hist.Samples())
}

// beep sounds the buzzer count times (1 when the body is empty). It answers
// 409 while another beep is still sounding. The JSON content type is required,
// so a cross-site form cannot trigger it.
func (s *server) beep(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)

		return
	}

	if !s.st.HasBuzzer() {
		http.Error(w, "no buzzer", http.StatusNotFound)

		return
	}

	req := beepRequest{Count: 1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)

		return
	}

	req.Count = min(max(req.Count, 1), maxBeeps)

	if !s.beeping.TryLock() {
		http.Error(w, "already beeping", http.StatusConflict)

		return
	}
	defer s.beeping.Unlock()

	for i := range req.Count {
		if err := s.st.Beep(beepOn); err != nil {
			slog.WarnContext(r.Context(), "beep failed", "err", err)
			http.Error(w, "beep failed", http.StatusInternalServerError)

			return
		}

		if i < req.Count-1 {
			time.Sleep(beepOn)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *server) lcd(w http.ResponseWriter, r *http.Request) {
	var req lcdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)

		return
	}

	lines := req.Lines
	if len(lines) == 0 && req.Text != "" {
		lines = strings.Split(req.Text, "\n")
	}

	if len(lines) == 0 {
		http.Error(w, "no text or lines", http.StatusBadRequest)

		return
	}

	ttl := defaultMessageTTL
	if req.Seconds > 0 {
		ttl = time.Duration(req.Seconds) * time.Second
	}

	s.msg.set(lines, ttl)
	w.WriteHeader(http.StatusNoContent)
}

func isJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))

	return err == nil && mt == "application/json"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("writing json response", "err", err)
	}
}
