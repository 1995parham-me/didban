package serve

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/1995parham-me/didban/internal/sensor"
	"github.com/1995parham-me/didban/internal/tunnel"
)

func lit(t *testing.T, snap *tunnel.Snapshot) int {
	t.Helper()

	temp := 33.8
	img := oledScreen(time.Date(2026, 11, 9, 18, 52, 7, 0, time.UTC), &sensor.Reading{TemperatureC: &temp}, snap, nil)

	if path := os.Getenv("OLED_PREVIEW"); path != "" {
		f, err := os.Create(path + "-" + t.Name() + ".png")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()

		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}

	n := 0

	for _, v := range img.Pix {
		if v != 0 {
			n++
		}
	}

	return n
}

func TestOLEDScreen(t *testing.T) {
	t.Parallel()

	d := 203
	up := lit(t, &tunnel.Snapshot{
		At: time.Now(), OK: true, Selector: "tunnel", Now: "auto", Exit: "htz02-hy2",
		Exits:   []tunnel.Exit{{Name: "htz02-hy2", Active: true, DelayMS: &d}},
		DownBps: 1.234e6, UpBps: 45600, Flows: make([]tunnel.Flow, 4),
	})
	failOpen := lit(t, &tunnel.Snapshot{At: time.Now(), OK: true, Now: "direct", Exit: "direct"})

	if up == 0 || failOpen <= up/2 {
		t.Fatalf("lit pixels: up %d, fail-open %d (the inverted bar should light more)", up, failOpen)
	}
}

func TestLEDPattern(t *testing.T) {
	t.Parallel()

	cases := []struct {
		snap *tunnel.Snapshot
		want string
	}{
		{nil, patternOff},
		{&tunnel.Snapshot{}, patternOff},
		{&tunnel.Snapshot{At: time.Now()}, patternDown},
		{&tunnel.Snapshot{At: time.Now(), OK: true, Now: "direct", Exit: "direct"}, patternFailOpen},
		{&tunnel.Snapshot{At: time.Now(), OK: true, Now: "auto", Exit: "htz02-hy2"}, patternUp},
		{&tunnel.Snapshot{At: time.Now(), OK: true, Now: "auto", Exit: "htz02-hy2", DownBps: 2e6}, patternBusy},
	}

	for _, c := range cases {
		if got, p := ledPattern(c.snap); got != c.want || len(p) == 0 {
			t.Errorf("%+v: got %s (%d ticks), want %s", c.snap, got, len(p), c.want)
		}
	}
}
