package sensor

import (
	"sync"
	"time"
)

// Sample is the part of a Reading worth keeping for a chart.
type Sample struct {
	At              time.Time `json:"t"`
	TemperatureC    *float64  `json:"temperature_c,omitempty"`
	PressurePa      *float64  `json:"pressure_pa,omitempty"`
	RTCTemperatureC *float64  `json:"rtc_temperature_c,omitempty"`
}

// History keeps the last size samples, at most one per step, in memory. It
// feeds the web page's charts; long-term history belongs to Prometheus. It is
// safe for concurrent use.
type History struct {
	mu   sync.Mutex
	step time.Duration
	size int
	buf  []Sample
}

// NewHistory returns a History holding size samples spaced at least step
// apart, so it covers size × step.
func NewHistory(size int, step time.Duration) *History {
	return &History{step: step, size: size, buf: make([]Sample, 0, size)}
}

// Add records r when at least step has passed since the last kept sample.
// A reading in which no chart value was measured is dropped.
func (h *History) Add(r Reading) {
	if r.TemperatureC == nil && r.PressurePa == nil && r.RTCTemperatureC == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if n := len(h.buf); n > 0 && r.At.Sub(h.buf[n-1].At) < h.step {
		return
	}

	if len(h.buf) == h.size {
		copy(h.buf, h.buf[1:])
		h.buf = h.buf[:h.size-1]
	}

	h.buf = append(h.buf, Sample{
		At:              r.At,
		TemperatureC:    r.TemperatureC,
		PressurePa:      r.PressurePa,
		RTCTemperatureC: r.RTCTemperatureC,
	})
}

// Samples returns a copy of the kept samples, oldest first.
func (h *History) Samples() []Sample {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]Sample, len(h.buf))
	copy(out, h.buf)

	return out
}
