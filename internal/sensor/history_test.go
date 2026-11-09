package sensor_test

import (
	"testing"
	"time"

	"github.com/1995parham-me/didban/internal/sensor"
)

func reading(at time.Time, temp float64) sensor.Reading {
	return sensor.Reading{At: at, TemperatureC: &temp}
}

func TestHistoryKeepsOnePerStep(t *testing.T) {
	t.Parallel()

	h := sensor.NewHistory(10, time.Minute)
	start := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	// A poll every 15 s for 3 minutes keeps one sample per minute.
	for i := range 13 {
		h.Add(reading(start.Add(time.Duration(i)*15*time.Second), float64(i)))
	}

	got := h.Samples()
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}

	for i, s := range got {
		if want := start.Add(time.Duration(i) * time.Minute); !s.At.Equal(want) {
			t.Errorf("sample %d at %v, want %v", i, s.At, want)
		}
	}
}

func TestHistoryDropsOldest(t *testing.T) {
	t.Parallel()

	h := sensor.NewHistory(3, time.Second)
	start := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	for i := range 5 {
		h.Add(reading(start.Add(time.Duration(i)*time.Second), float64(i)))
	}

	got := h.Samples()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	if *got[0].TemperatureC != 2 || *got[2].TemperatureC != 4 {
		t.Errorf("kept %v .. %v, want 2 .. 4", *got[0].TemperatureC, *got[2].TemperatureC)
	}
}

func TestHistorySkipsEmptyReadings(t *testing.T) {
	t.Parallel()

	h := sensor.NewHistory(3, time.Second)
	h.Add(sensor.Reading{At: time.Now()})

	if n := len(h.Samples()); n != 0 {
		t.Errorf("len = %d, want 0", n)
	}
}
