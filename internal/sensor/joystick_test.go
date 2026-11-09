package sensor_test

import (
	"slices"
	"testing"
	"time"

	"github.com/1995parham-me/didban/internal/sensor"
)

func TestPressesCountOncePerPress(t *testing.T) {
	t.Parallel()

	p := sensor.NewPresses()
	t0 := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	steps := []struct {
		held []sensor.Key
		want []sensor.Key
	}{
		{nil, nil},
		{[]sensor.Key{sensor.KeyUp}, []sensor.Key{sensor.KeyUp}},
		{[]sensor.Key{sensor.KeyUp}, nil}, // held: no new press
		{[]sensor.Key{sensor.KeyUp, sensor.KeyCenter}, []sensor.Key{sensor.KeyCenter}},
		{nil, nil},
		{[]sensor.Key{sensor.KeyUp}, []sensor.Key{sensor.KeyUp}}, // released and pressed again
	}

	for i, s := range steps {
		got := p.Update(t0.Add(time.Duration(i)*50*time.Millisecond), s.held)
		if !slices.Equal(got, s.want) {
			t.Errorf("step %d: pressed %v, want %v", i, got, s.want)
		}
	}

	st := p.State()
	if st.Counts[sensor.KeyUp] != 2 || st.Counts[sensor.KeyCenter] != 1 || st.Counts[sensor.KeyDown] != 0 {
		t.Errorf("counts = %v", st.Counts)
	}

	if len(st.Recent) != 3 || st.Recent[0].Key != sensor.KeyUp || st.Recent[1].Key != sensor.KeyCenter {
		t.Errorf("recent = %v, want newest first", st.Recent)
	}

	if !slices.Equal(st.Pressed, []sensor.Key{sensor.KeyUp}) {
		t.Errorf("pressed = %v", st.Pressed)
	}
}

func TestPressesKeepsTheLastTen(t *testing.T) {
	t.Parallel()

	p := sensor.NewPresses()
	t0 := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	for i := range 30 {
		held := []sensor.Key{}
		if i%2 == 0 {
			held = []sensor.Key{sensor.KeyLeft}
		}

		p.Update(t0.Add(time.Duration(i)*time.Second), held)
	}

	st := p.State()
	if len(st.Recent) != 10 || st.Counts[sensor.KeyLeft] != 15 {
		t.Errorf("recent %d, count %d", len(st.Recent), st.Counts[sensor.KeyLeft])
	}

	if !st.Recent[0].At.Equal(t0.Add(28 * time.Second)) {
		t.Errorf("newest at %v", st.Recent[0].At)
	}
}
