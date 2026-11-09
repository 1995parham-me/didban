package serve

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/1995parham-me/didban/internal/infra/ssd1306"
	"github.com/1995parham-me/didban/internal/sensor"
	"github.com/1995parham-me/didban/internal/tunnel"
)

const (
	// oledRefresh is how often the OLED is redrawn: the clock and rates move.
	oledRefresh = time.Second

	// oledCols is how many 7-pixel characters fit on a 128-pixel row.
	oledCols = 18
)

// oledRows are the text baselines of the four 16-pixel rows.
var oledRows = [4]int{11, 27, 43, 59}

// drawOLED redraws the OLED every second until ctx is done, then blanks it.
func (s *server) drawOLED(ctx context.Context, d *ssd1306.Display) {
	ticker := time.NewTicker(oledRefresh)
	defer ticker.Stop()

	defer func() {
		if err := d.Close(); err != nil {
			slog.Warn("closing oled", "err", err)
		}
	}()

	failing := false

	for {
		now := time.Now()

		var snap *tunnel.Snapshot
		if s.tunnel != nil {
			t := s.tunnel.Snapshot()
			snap = &t
		}

		img := oledScreen(now.In(s.cfg.Location()), s.now.get(), snap, s.msg.active(now))

		if err := d.Show(img); err != nil {
			if !failing {
				slog.WarnContext(ctx, "oled update failed", "err", err)
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

// oledScreen lays out the 128×64 status screen:
//
//	18:52:07    33.8 C     clock and temperature
//	htz02-hy2     203ms    the tunnel's exit, inverted when it is not healthy
//	down   1.23 MB/s       tunnel throughput
//	up     45.6 kB/s   4f  and the open flows
//
// Without a tunnel the last three rows show the pressure. A message pushed to
// POST /lcd replaces the screen while it lasts. The whole screen moves by a
// pixel each minute, so no pixel stays lit for days (OLEDs burn in).
func oledScreen(now time.Time, r *sensor.Reading, t *tunnel.Snapshot, msg []string) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, ssd1306.Width, ssd1306.Height))
	shift := now.Minute() % 3

	if msg != nil {
		for i, line := range msg {
			if i < len(oledRows) {
				oledText(img, shift, i, line, false)
			}
		}

		return img
	}

	temp := ""

	if r != nil {
		switch {
		case len(r.Probes) > 0:
			temp = fmt.Sprintf("%.1f C", r.Probes[0].Celsius)
		case r.TemperatureC != nil:
			temp = fmt.Sprintf("%.1f C", *r.TemperatureC)
		}
	}

	oledText(img, shift, 0, spread(now.Format("15:04:05"), temp), false)

	if t == nil {
		if r != nil && r.PressurePa != nil {
			oledText(img, shift, 1, fmt.Sprintf("%.1f hPa", *r.PressurePa/100), false)
		}

		return img
	}

	switch {
	case t.At.IsZero():
		oledText(img, shift, 1, "tunnel: waiting", false)
	case !t.OK:
		oledText(img, shift, 1, "TUNNEL API DOWN", true)
	case t.FailOpen():
		oledText(img, shift, 1, "FAIL-OPEN: direct", true)
	default:
		delay := ""

		for _, e := range t.Exits {
			if e.Active && e.DelayMS != nil && *e.DelayMS > 0 {
				delay = fmt.Sprintf("%dms", *e.DelayMS)
			}
		}

		oledText(img, shift, 1, spread(t.Exit, delay), false)
	}

	if !t.OK {
		return img
	}

	oledText(img, shift, 2, "down "+oledRate(t.DownBps), false)
	oledText(img, shift, 3, spread("up   "+oledRate(t.UpBps), fmt.Sprintf("%d fl", len(t.Flows))), false)

	return img
}

// oledText writes one row; inverted rows are a lit bar with dark text.
func oledText(img *image.Gray, shift, row int, s string, inverted bool) {
	fg, bg := color.Gray{Y: 0xFF}, color.Gray{Y: 0}
	if inverted {
		fg, bg = bg, fg
		top := oledRows[row] - 12
		draw.Draw(img, image.Rect(0, top, ssd1306.Width, top+15), image.NewUniform(bg), image.Point{}, draw.Src)
	}

	if len(s) > oledCols {
		s = s[:oledCols]
	}

	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(fg),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(shift, oledRows[row]),
	}
	d.DrawString(s)
}

// spread puts left and right at the two ends of a row.
func spread(left, right string) string {
	gap := max(oledCols-1-len(left)-len(right), 1)

	return left + strings.Repeat(" ", gap) + right
}

// oledRate formats bytes a second in 9 characters, e.g. "  1.23 MB/s".
func oledRate(bps float64) string {
	units := []string{"B/s", "kB/s", "MB/s", "GB/s"}
	i := 0

	for bps >= 1000 && i < len(units)-1 {
		bps /= 1000
		i++
	}

	if i == 0 {
		return fmt.Sprintf("%6.0f %s", bps, units[i])
	}

	return fmt.Sprintf("%6.2f %s", bps, units[i])
}
