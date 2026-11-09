package ds3231

import "testing"

func TestBCD(t *testing.T) {
	t.Parallel()

	for in, want := range map[byte]int{0x00: 0, 0x09: 9, 0x10: 10, 0x59: 59, 0x23: 23} {
		if got := bcd(in); got != want {
			t.Errorf("bcd(0x%02x) = %d, want %d", in, got, want)
		}
	}
}
