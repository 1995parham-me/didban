package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/1995parham-me/didban/internal/sensor"
)

// defaultMessageTTL is how long a pushed LCD message stays up when the request
// does not set a duration.
const defaultMessageTTL = 60 * time.Second

// message is the custom text currently pinned to the LCD, with an expiry. The
// poller shows it instead of the status screen while active.
type message struct {
	mu    sync.Mutex
	lines []string
	until time.Time
}

// set pins lines for ttl.
func (m *message) set(lines []string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lines = lines
	m.until = time.Now().Add(ttl)
}

// active returns the pinned lines if they have not expired, else nil.
func (m *message) active(now time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if now.After(m.until) {
		return nil
	}

	return m.lines
}

// lcdRequest is the POST /lcd body: either text (split on newlines) or an
// explicit list of lines, held for seconds (0 → defaultMessageTTL).
type lcdRequest struct {
	Text    string   `json:"text"`
	Lines   []string `json:"lines"`
	Seconds int      `json:"seconds"`
}

// handler builds the HTTP mux: /metrics for Prometheus, POST /lcd for messages.
func handler(reg *prometheus.Registry, msg *message) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	mux.HandleFunc("POST /lcd", func(w http.ResponseWriter, r *http.Request) {
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

		msg.set(lines, ttl)
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

// statusLines renders the default LCD screen from a reading: time + temperature
// on the first row, pressure on the second.
func statusLines(r sensor.Reading, cols int) []string {
	clock := "--:--:--"
	if r.RTCTime != nil {
		clock = r.RTCTime.Format("15:04:05")
	}

	temp := ""
	if r.TemperatureC != nil {
		temp = fmt.Sprintf("%.1fC", *r.TemperatureC)
	} else if r.RTCTemperatureC != nil {
		temp = fmt.Sprintf("%.1fC", *r.RTCTemperatureC)
	}

	top := clock
	if temp != "" {
		top = fit(clock+" "+temp, cols)
	}

	bottom := ""
	if r.PressurePa != nil {
		bottom = fmt.Sprintf("P %.0f hPa", *r.PressurePa/100.0)
	}

	return []string{top, bottom}
}

// fit pads or truncates s to width with a left clock / right value layout kept
// simple: just cap the length.
func fit(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}

	return s
}
