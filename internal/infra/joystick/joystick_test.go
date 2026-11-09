package joystick

import (
	"slices"
	"testing"

	"github.com/1995parham-me/didban/internal/sensor"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	cases := map[byte][]sensor.Key{
		0xff: nil,                             // idle: everything high
		0x7f: nil,                             // buzzer sounding (bit 7 low) is not a key
		0xef: nil,                             // LED2 on (bit 4 low) is not a key
		0xfe: {sensor.KeyLeft},                // Waveshare: (value | 0xFE) != 0xFF → left
		0xfd: {sensor.KeyUp},                  // 0xFD → up
		0xfb: {sensor.KeyDown},                // 0xFB → down
		0xf7: {sensor.KeyRight},               // otherwise → right
		0xf5: {sensor.KeyUp, sensor.KeyRight}, // two at once
		0x70: {sensor.KeyLeft, sensor.KeyUp, sensor.KeyDown, sensor.KeyRight},
	}

	for port, want := range cases {
		if got := decode(port); !slices.Equal(got, want) {
			t.Errorf("decode(0x%02x) = %v, want %v", port, got, want)
		}
	}
}
