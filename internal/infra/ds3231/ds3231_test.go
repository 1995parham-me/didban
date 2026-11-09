package ds3231

import (
	"testing"
	"time"
)

func TestBCD(t *testing.T) {
	t.Parallel()

	for in, want := range map[byte]int{0x00: 0, 0x09: 9, 0x10: 10, 0x59: 59, 0x23: 23} {
		if got := bcd(in); got != want {
			t.Errorf("bcd(0x%02x) = %d, want %d", in, got, want)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		t.Skip("no tzdata:", err)
	}

	for _, want := range []time.Time{
		time.Date(2026, 11, 9, 9, 30, 5, 0, loc),
		time.Date(2000, 1, 1, 0, 0, 0, 0, loc),
		time.Date(2099, 12, 31, 23, 59, 59, 0, loc),
	} {
		if got := decode(encode(want), loc); !got.Equal(want) {
			t.Errorf("round trip %v = %v", want, got)
		}
	}
}

func TestEncodeRegisters(t *testing.T) {
	t.Parallel()

	// Monday 2026-11-09 09:30:05: day of week 2 (Sunday is 1).
	got := encode(time.Date(2026, 11, 9, 9, 30, 5, 0, time.UTC))
	want := [7]byte{0x05, 0x30, 0x09, 0x02, 0x09, 0x11, 0x26}

	if got != want {
		t.Errorf("encode = % x, want % x", got, want)
	}
}
