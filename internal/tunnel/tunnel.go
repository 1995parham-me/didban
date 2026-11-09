// Package tunnel watches a sing-box tunnel through its Clash API: which exit
// the selector uses now, how fast each exit answers its probe, the throughput,
// and the flows open right now with their own rates. A Watcher polls on an
// interval and keeps the newest Snapshot plus a per-minute throughput history
// for the status page.
package tunnel

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/1995parham-me/didban/internal/infra/clashapi"
)

// Source is the part of the Clash API the watcher reads.
type Source interface {
	Connections(ctx context.Context) (clashapi.Connections, error)
	Proxies(ctx context.Context) (map[string]clashapi.Proxy, error)
}

// Exit is one outbound the selector can reach, with its last probe.
type Exit struct {
	Name    string     `json:"name"`
	Type    string     `json:"type"`
	DelayMS *int       `json:"delay_ms,omitempty"`
	At      *time.Time `json:"at,omitempty"`
	Active  bool       `json:"active"`
}

// Flow is one open connection through the tunnel.
type Flow struct {
	ID       string    `json:"id"`
	Network  string    `json:"network"`
	Host     string    `json:"host"`
	Dest     string    `json:"dest"`
	Source   string    `json:"source"`
	Exit     string    `json:"exit"`
	Start    time.Time `json:"start"`
	Download int64     `json:"download"`
	Upload   int64     `json:"upload"`
	DownBps  float64   `json:"down_bps"`
	UpBps    float64   `json:"up_bps"`
}

// Snapshot is the newest poll. Rates are bytes per second over the last poll.
type Snapshot struct {
	At            time.Time `json:"at"`
	OK            bool      `json:"ok"`
	Error         string    `json:"error,omitempty"`
	Selector      string    `json:"selector"`
	Now           string    `json:"now"`
	Exit          string    `json:"exit"`
	Exits         []Exit    `json:"exits"`
	DownloadTotal int64     `json:"download_total"`
	UploadTotal   int64     `json:"upload_total"`
	DownBps       float64   `json:"down_bps"`
	UpBps         float64   `json:"up_bps"`
	Flows         []Flow    `json:"flows"`
}

// Point is one minute of throughput, averaged over the minute.
type Point struct {
	At      time.Time `json:"t"`
	DownBps float64   `json:"down_bps"`
	UpBps   float64   `json:"up_bps"`
}

// Watcher polls a Source. It is safe for concurrent use.
type Watcher struct {
	src      Source
	selector string
	size     int

	mu      sync.Mutex
	snap    Snapshot
	prev    map[string]clashapi.Connection
	prevAt  time.Time
	hist    []Point
	bucket  time.Time
	bucketD int64
	bucketU int64
}

// New returns a watcher of the selector group named selector, keeping size
// minutes of history.
func New(src Source, selector string, size int) *Watcher {
	return &Watcher{src: src, selector: selector, size: size, snap: Snapshot{Selector: selector}}
}

// Run polls every interval until ctx is done.
func (w *Watcher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	failing := false

	for {
		if err := w.Poll(ctx); err != nil {
			if !failing {
				slog.WarnContext(ctx, "tunnel api failed", "err", err)
			}

			failing = true
		} else {
			failing = false
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Snapshot returns the newest poll.
func (w *Watcher) Snapshot() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()

	s := w.snap
	s.Exits = slices.Clone(s.Exits)
	s.Flows = slices.Clone(s.Flows)

	return s
}

// History returns the per-minute throughput, oldest first.
func (w *Watcher) History() []Point {
	w.mu.Lock()
	defer w.mu.Unlock()

	return slices.Clone(w.hist)
}

// Poll reads the API once and updates the snapshot.
func (w *Watcher) Poll(ctx context.Context) error {
	now := time.Now()

	conns, cErr := w.src.Connections(ctx)

	proxies, pErr := w.src.Proxies(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := errors.Join(cErr, pErr); err != nil {
		w.snap.At = now
		w.snap.OK = false
		w.snap.Error = err.Error()
		w.snap.DownBps, w.snap.UpBps = 0, 0
		w.snap.Flows = nil
		w.prev = nil

		return fmt.Errorf("tunnel api: %w", err)
	}

	snap := Snapshot{
		At:            now,
		OK:            true,
		Selector:      w.selector,
		DownloadTotal: conns.DownloadTotal,
		UploadTotal:   conns.UploadTotal,
	}

	snap.Now = proxies[w.selector].Now
	snap.Exit = resolve(proxies, w.selector)
	snap.Exits = exits(proxies, w.selector, snap.Exit)

	dt := now.Sub(w.prevAt).Seconds()
	if w.prev != nil && dt > 0 {
		d := max(conns.DownloadTotal-w.snap.DownloadTotal, 0)
		u := max(conns.UploadTotal-w.snap.UploadTotal, 0)
		snap.DownBps = float64(d) / dt
		snap.UpBps = float64(u) / dt
		w.account(now, d, u)
	}

	next := make(map[string]clashapi.Connection, len(conns.Connections))
	snap.Flows = make([]Flow, 0, len(conns.Connections))

	for _, c := range conns.Connections {
		next[c.ID] = c
		snap.Flows = append(snap.Flows, flow(c, w.prev[c.ID], w.prev != nil, dt, now))
	}

	slices.SortFunc(snap.Flows, func(a, b Flow) int {
		if r := cmp.Compare(b.DownBps+b.UpBps, a.DownBps+a.UpBps); r != 0 {
			return r
		}

		return cmp.Compare(b.Download+b.Upload, a.Download+a.Upload)
	})

	w.snap = snap
	w.prev = next
	w.prevAt = now

	return nil
}

// account adds bytes to the current minute and closes the minute when a new
// one starts.
func (w *Watcher) account(now time.Time, down, up int64) {
	minute := now.Truncate(time.Minute)
	if !w.bucket.IsZero() && !minute.Equal(w.bucket) {
		w.hist = append(w.hist, Point{
			At:      w.bucket,
			DownBps: float64(w.bucketD) / 60,
			UpBps:   float64(w.bucketU) / 60,
		})
		if len(w.hist) > w.size {
			w.hist = w.hist[len(w.hist)-w.size:]
		}

		w.bucketD, w.bucketU = 0, 0
	}

	w.bucket = minute
	w.bucketD += down
	w.bucketU += up
}

func flow(c, prev clashapi.Connection, havePrev bool, dt float64, now time.Time) Flow {
	f := Flow{
		ID:       c.ID,
		Network:  c.Metadata.Network,
		Host:     c.Metadata.Host,
		Dest:     c.Metadata.DestinationIP + ":" + c.Metadata.DestinationPort,
		Source:   c.Metadata.SourceIP,
		Start:    c.Start,
		Download: c.Download,
		Upload:   c.Upload,
	}

	if len(c.Chains) > 0 {
		f.Exit = c.Chains[0]
	}

	// A flow opened since the last poll is measured from its start.
	span := dt
	if prev.ID == "" {
		span = now.Sub(c.Start).Seconds()
	}

	if havePrev && span > 0 {
		f.DownBps = float64(max(c.Download-prev.Download, 0)) / span
		f.UpBps = float64(max(c.Upload-prev.Upload, 0)) / span
	}

	return f
}

// resolve follows group picks from name down to an outbound.
func resolve(proxies map[string]clashapi.Proxy, name string) string {
	for range 8 {
		p, ok := proxies[name]
		if !ok || p.Now == "" {
			return name
		}

		name = p.Now
	}

	return name
}

// exits lists the outbounds reachable from the selector, groups expanded, in
// the selector's order.
func exits(proxies map[string]clashapi.Proxy, selector, active string) []Exit {
	var out []Exit

	seen := map[string]bool{}

	var walk func(name string, depth int)

	walk = func(name string, depth int) {
		if seen[name] || depth > 8 {
			return
		}

		seen[name] = true

		p := proxies[name]
		if len(p.All) > 0 {
			for _, m := range p.All {
				walk(m, depth+1)
			}

			return
		}

		e := Exit{Name: name, Type: p.Type, Active: name == active}
		if d := p.LastDelay(); d != nil && p.Type != "Direct" {
			e.DelayMS = &d.Delay
			e.At = &d.Time
		}

		out = append(out, e)
	}

	walk(selector, 0)

	return out
}
