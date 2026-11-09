// Package station composes the individual I²C drivers into one sensor station:
// it opens the bus, builds the enabled sensors and the LCD, reads them all
// into a sensor.Reading, and renders text to the display. Per-sensor failures
// are logged and skipped rather than failing the whole read, so one dead chip
// does not blind the others.
package station

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"periph.io/x/conn/v3/i2c"

	"github.com/1995parham-me/didban/internal/config"
	"github.com/1995parham-me/didban/internal/infra/bmp180"
	"github.com/1995parham-me/didban/internal/infra/buzzer"
	"github.com/1995parham-me/didban/internal/infra/ds3231"
	"github.com/1995parham-me/didban/internal/infra/i2cbus"
	"github.com/1995parham-me/didban/internal/infra/lcd"
	"github.com/1995parham-me/didban/internal/infra/pcf8591"
	"github.com/1995parham-me/didban/internal/sensor"
)

// Station is a live sensor HAT. Build it with New and release the bus with
// Close. It is safe for concurrent use: every bus access holds mu, so a beep
// from the web page cannot interleave with a sensor poll.
type Station struct {
	mu   sync.Mutex
	cfg  config.Config
	bus  i2c.BusCloser
	bmp  *bmp180.Sensor
	rtc  *ds3231.Clock
	adc  *pcf8591.ADC
	lcd  *lcd.Display
	buzz *buzzer.Buzzer
	loc  *time.Location
}

// New opens the bus and constructs every enabled component. A sensor that
// fails to initialize is logged and left out rather than failing New, so the
// station still serves whatever hardware is present.
func New(cfg config.Config) (*Station, error) {
	bus, err := i2cbus.Open(cfg.I2C.Bus)
	if err != nil {
		return nil, err
	}

	s := &Station{cfg: cfg, bus: bus, loc: cfg.Location()}

	if cfg.BMP180.Enabled {
		if bmp, err := bmp180.New(i2cbus.Dev(bus, cfg.BMP180.Address)); err != nil {
			slog.Warn("bmp180 unavailable", "err", err)
		} else {
			s.bmp = bmp
		}
	}

	if cfg.DS3231.Enabled {
		s.rtc = ds3231.New(i2cbus.Dev(bus, cfg.DS3231.Address))
	}

	if cfg.ADC.Enabled {
		s.adc = pcf8591.New(i2cbus.Dev(bus, cfg.ADC.Address))
	}

	if cfg.Buzzer.Enabled {
		// New leaves the buzzer silent; a failure here only costs the beep
		// feature, so log and continue.
		if bz, err := buzzer.New(i2cbus.Dev(bus, cfg.Buzzer.Address), cfg.Buzzer.Bit); err != nil {
			slog.Warn("buzzer unavailable", "err", err)
		} else {
			s.buzz = bz
		}
	}

	if cfg.LCD.Enabled {
		d := lcd.New(i2cbus.Dev(bus, cfg.LCD.Address), cfg.LCD.Cols, cfg.LCD.Rows)
		d.Backlight(cfg.LCD.Backlight)
		if err := d.Init(); err != nil {
			slog.Warn("lcd unavailable", "err", err)
		} else {
			s.lcd = d
		}
	}

	return s, nil
}

// Read polls every available sensor once.
func (s *Station) Read(ctx context.Context) sensor.Reading {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := sensor.Reading{At: time.Now()}

	if s.bmp != nil {
		if t, p, err := s.bmp.Read(); err != nil {
			slog.WarnContext(ctx, "bmp180 read failed", "err", err)
		} else {
			r.TemperatureC, r.PressurePa = &t, &p
		}
	}

	if s.rtc != nil {
		if t, err := s.rtc.Temperature(); err != nil {
			slog.WarnContext(ctx, "ds3231 temperature read failed", "err", err)
		} else {
			r.RTCTemperatureC = &t
		}

		if now, err := s.rtc.Time(s.loc); err != nil {
			slog.WarnContext(ctx, "ds3231 time read failed", "err", err)
		} else {
			r.RTCTime = &now
		}
	}

	if s.adc != nil {
		if raw, err := s.adc.Read(); err != nil {
			slog.WarnContext(ctx, "pcf8591 read failed", "err", err)
		} else {
			for i, v := range raw {
				r.ADC = append(r.ADC, sensor.ADCChannel{
					Index: i,
					Label: s.label(i),
					Raw:   v,
					Volts: float64(v) / 255.0 * s.cfg.ADC.VRef,
				})
			}
		}
	}

	return r
}

// Sensors reports which components were found and are in use, by name.
func (s *Station) Sensors() map[string]bool {
	return map[string]bool{
		"bmp180":  s.bmp != nil,
		"ds3231":  s.rtc != nil,
		"pcf8591": s.adc != nil,
		"buzzer":  s.buzz != nil,
		"lcd":     s.lcd != nil,
	}
}

// HasBuzzer reports whether a usable buzzer was found.
func (s *Station) HasBuzzer() bool { return s.buzz != nil }

// HasLCD reports whether a usable LCD was found.
func (s *Station) HasLCD() bool { return s.lcd != nil }

// ShowLines writes lines to the LCD, one per row. It is a no-op when no LCD
// was found.
func (s *Station) ShowLines(lines []string) error {
	if s.lcd == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.lcd.Lines(lines); err != nil {
		return fmt.Errorf("station: write lcd: %w", err)
	}

	return nil
}

// Beep sounds the buzzer for d. It is a no-op when no buzzer is configured.
func (s *Station) Beep(d time.Duration) error {
	if s.buzz == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.buzz.Beep(d); err != nil {
		return fmt.Errorf("station: beep: %w", err)
	}

	return nil
}

// SetClock writes t to the DS3231 in the configured timezone.
func (s *Station) SetClock(t time.Time) error {
	if s.rtc == nil {
		return errors.New("station: no ds3231 configured")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.rtc.SetTime(t, s.loc); err != nil {
		return fmt.Errorf("station: set clock: %w", err)
	}

	return nil
}

// Close silences the buzzer and releases the bus.
func (s *Station) Close() error {
	if s.buzz != nil {
		if err := s.buzz.Silence(); err != nil {
			slog.Warn("silencing buzzer on close", "err", err)
		}
	}

	if err := s.bus.Close(); err != nil {
		return fmt.Errorf("station: close bus: %w", err)
	}

	return nil
}

func (s *Station) label(i int) string {
	if i < len(s.cfg.ADC.Labels) {
		return s.cfg.ADC.Labels[i]
	}

	return fmt.Sprintf("ain%d", i)
}
