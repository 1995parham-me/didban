// Package ds3231 drives a Maxim DS3231 temperature-compensated real-time
// clock over I²C (address 0x68). It reads and sets the kept wall-clock time
// and reads the die temperature the chip exposes at 0x11–0x12.
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
	regStat = 0x0f // status; bit 7 (OSF) is set when the oscillator stopped.

	oscStopped = 0x80
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

	return decode(b, loc), nil
}

// SetTime writes t, as wall-clock time in loc, to the RTC and clears the
// oscillator-stopped flag, so the chip reports the clock as valid.
func (c *Clock) SetTime(t time.Time, loc *time.Location) error {
	b := encode(t.In(loc))
	if err := c.dev.Tx(append([]byte{regTime}, b[:]...), nil); err != nil {
		return fmt.Errorf("ds3231: write time: %w", err)
	}

	var st [1]byte
	if err := c.dev.Tx([]byte{regStat}, st[:]); err != nil {
		return fmt.Errorf("ds3231: read status: %w", err)
	}

	if err := c.dev.Tx([]byte{regStat, st[0] &^ oscStopped}, nil); err != nil {
		return fmt.Errorf("ds3231: clear oscillator flag: %w", err)
	}

	return nil
}

// decode turns the seven time registers into a time in loc. It assumes
// 24-hour mode, which encode always writes.
func decode(b [7]byte, loc *time.Location) time.Time {
	sec := bcd(b[0] & 0x7f)
	minute := bcd(b[1] & 0x7f)
	hour := bcd(b[2] & 0x3f)
	day := bcd(b[4] & 0x3f)
	month := bcd(b[5] & 0x1f)
	year := 2000 + bcd(b[6])

	return time.Date(year, time.Month(month), day, hour, minute, sec, 0, loc)
}

// encode is the inverse of decode: 24-hour mode, day of week 1 (Sunday) to 7,
// years 2000–2099.
func encode(t time.Time) [7]byte {
	return [7]byte{
		tobcd(t.Second()),
		tobcd(t.Minute()),
		tobcd(t.Hour()),
		tobcd(int(t.Weekday()) + 1),
		tobcd(t.Day()),
		tobcd(int(t.Month())),
		tobcd(t.Year() % 100),
	}
}

// tobcd encodes 0–99 as binary-coded decimal.
func tobcd(v int) byte {
	return byte(v/10<<4 | v%10) //nolint:gosec // v is a clock field, 0–99.
}

// bcd decodes a binary-coded-decimal byte.
func bcd(v byte) int {
	return int(v>>4)*10 + int(v&0x0f)
}
