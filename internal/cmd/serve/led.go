package serve

import (
	"context"
	"log/slog"
	"time"

	"github.com/1995parham-me/didban/internal/infra/led"
	"github.com/1995parham-me/didban/internal/tunnel"
)

// ledTick is the LED's time step; every pattern is a sequence of ticks.
const ledTick = 100 * time.Millisecond

// LED patterns, one bit per tick, oldest first, repeated.
const (
	patternOff      = "off"
	patternUp       = "up"        // steady on: the tunnel goes through htz-02
	patternBusy     = "busy"      // on, flickering faster the more data moves
	patternFailOpen = "fail-open" // slow blink: the selector is on direct
	patternDown     = "api-down"  // double flash: sing-box's API does not answer
)

var ledPatterns = map[string][]bool{
	patternOff:      {false},
	patternUp:       {true},
	patternFailOpen: {true, true, true, true, true, true, true, true, true, true, false, false, false, false, false, false, false, false, false, false},
	patternDown:     {true, false, true, false, false, false, false, false, false, false},
}

// ledPattern picks the LED's pattern from the tunnel's state.
func ledPattern(t *tunnel.Snapshot) (string, []bool) {
	switch {
	case t == nil || t.At.IsZero():
		return patternOff, ledPatterns[patternOff]
	case !t.OK:
		return patternDown, ledPatterns[patternDown]
	case t.FailOpen():
		return patternFailOpen, ledPatterns[patternFailOpen]
	}

	// Like a network card's light: off for one tick every n.
	n := 0

	switch rate := t.DownBps + t.UpBps; {
	case rate >= 1e6:
		n = 2
	case rate >= 1e5:
		n = 3
	case rate >= 1e3:
		n = 6
	}

	if n == 0 {
		return patternUp, ledPatterns[patternUp]
	}

	p := make([]bool, n)
	for i := 1; i < n; i++ {
		p[i] = true
	}

	return patternBusy, p
}

// driveLED shows the tunnel's state on the LED until ctx is done, then turns
// it off.
func (s *server) driveLED(ctx context.Context, l *led.LED) {
	ticker := time.NewTicker(ledTick)
	defer ticker.Stop()

	defer func() {
		if err := l.Set(false); err != nil {
			slog.Warn("turning the led off", "err", err)
		}
	}()

	failing := false

	for i := 0; ; i++ {
		var snap *tunnel.Snapshot
		if s.tunnel != nil {
			t := s.tunnel.Snapshot()
			snap = &t
		}

		name, p := ledPattern(snap)
		s.ledState.Store(name)

		if err := l.Set(p[i%len(p)]); err != nil {
			if !failing {
				slog.WarnContext(ctx, "led update failed", "err", err)
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
