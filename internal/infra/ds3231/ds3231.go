// Package ds3231 drives a Maxim DS3231 temperature-compensated real-time
// clock over I²C (address 0x68). It reads the kept wall-clock time and the
// die temperature the chip exposes at 0x11–0x12.
package ds3231

import (
	"fmt"
	"time"

	"periph.io/x/conn/v3/i2c"
)

// Addr is the DS3231's fixed I²C address.
const Addr uint16 = 0x68

const (
	regTime = 0x00 // 7 BCD bytes: sec, min, hour, dow, day, month, year.
	regTemp = 0x11 // 2 bytes: signed integer °C + quarter-degree fraction.
)

// Clock is a DS3231 on a bus.
type Clock struct {
	dev *i2c.Dev
}

// New returns a DS3231 handle. It does not probe; call Temperature or Time to
// confirm the chip answers.
func New(dev *i2c.Dev) *Clock {
	return &Clock{dev: dev}
}

// Temperature returns the die temperature in °C (0.25 °C resolution).
func (c *Clock) Temperature() (float64, error) {
	var buf [2]byte
	if err := c.dev.Tx([]byte{regTemp}, buf[:]); err != nil {
		return 0, fmt.Errorf("ds3231: read temperature: %w", err)
	}

	// buf[0] is a signed integer °C; the top two bits of buf[1] are the
	// fractional quarters.
	return float64(int8(buf[0])) + float64(buf[1]>>6)*0.25, nil //nolint:gosec // two's complement by design.
}

// Time returns the wall-clock time the RTC is holding, interpreted in loc.
func (c *Clock) Time(loc *time.Location) (time.Time, error) {
	var b [7]byte
	if err := c.dev.Tx([]byte{regTime}, b[:]); err != nil {
		return time.Time{}, fmt.Errorf("ds3231: read time: %w", err)
	}

	sec := bcd(b[0] & 0x7f)
	minute := bcd(b[1] & 0x7f)
	hour := bcd(b[2] & 0x3f) // assume 24-hour mode (bit 6 clear).
	day := bcd(b[4] & 0x3f)
	month := bcd(b[5] & 0x1f)
	year := 2000 + bcd(b[6])

	return time.Date(year, time.Month(month), day, hour, minute, sec, 0, loc), nil
}

// bcd decodes a binary-coded-decimal byte.
func bcd(v byte) int {
	return int(v>>4)*10 + int(v&0x0f)
}
