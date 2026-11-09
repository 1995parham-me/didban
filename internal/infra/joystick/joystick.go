// Package joystick reads the Pioneer600's five-way joystick. The four
// directions are inputs on the PCF8574 I/O expander (0x20) and the centre
// press is a Pi GPIO; all five read low while pressed. The wiring follows
// Waveshare's own demo code (PCF8574/python/pcf8574.py, KEY/python/key.py):
// bit 0 left, bit 1 up, bit 2 down, bit 3 right, centre on BCM 20 with a
// pull-up.
//
// The PCF8574 is quasi-bidirectional: a pin reads as an input only while it
// is latched high. The buzzer driver on the same chip keeps bits 0–3 high in
// every value it writes, so reading here needs no write first.
package joystick

import (
	"fmt"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/i2c"

	"github.com/1995parham-me/didban/internal/sensor"
)

// directions maps PCF8574 bits to keys.
var directions = []struct {
	bit byte
	key sensor.Key
}{
	{0x01, sensor.KeyLeft},
	{0x02, sensor.KeyUp},
	{0x04, sensor.KeyDown},
	{0x08, sensor.KeyRight},
}

// Joystick is the five-way switch.
type Joystick struct {
	dev    *i2c.Dev
	center gpio.PinIn
}

// New returns the joystick on the PCF8574 dev, with the centre key on the GPIO
// named center (periph's name, e.g. "GPIO20"). An empty or unknown name leaves
// the centre key out and is reported as an error, so the caller can warn and
// keep the four directions.
func New(dev *i2c.Dev, center string) (*Joystick, error) {
	j := &Joystick{dev: dev}

	if center == "" {
		return j, nil
	}

	pin := gpioreg.ByName(center)
	if pin == nil {
		return j, fmt.Errorf("joystick: no gpio named %q", center)
	}

	if err := pin.In(gpio.PullUp, gpio.NoEdge); err != nil {
		return j, fmt.Errorf("joystick: set up %s: %w", center, err)
	}

	j.center = pin

	return j, nil
}

// Held returns the keys pressed now.
func (j *Joystick) Held() ([]sensor.Key, error) {
	var port [1]byte
	if err := j.dev.Tx(nil, port[:]); err != nil {
		return nil, fmt.Errorf("joystick: read pcf8574: %w", err)
	}

	held := decode(port[0])

	if j.center != nil && j.center.Read() == gpio.Low {
		held = append(held, sensor.KeyCenter)
	}

	return held, nil
}

// decode returns the directions whose bits are low in a PCF8574 port value.
func decode(port byte) []sensor.Key {
	var held []sensor.Key

	for _, d := range directions {
		if port&d.bit == 0 {
			held = append(held, d.key)
		}
	}

	return held
}
