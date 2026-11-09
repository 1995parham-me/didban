// Package led drives a single LED on a Pi GPIO (active high), such as the
// Pioneer600's LED1 on BCM 26.
package led

import (
	"fmt"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
)

// LED is one light.
type LED struct {
	pin gpio.PinOut
}

// New returns the LED on the GPIO named pin (periph's name, e.g. "GPIO26"),
// switched off.
func New(pin string) (*LED, error) {
	p := gpioreg.ByName(pin)
	if p == nil {
		return nil, fmt.Errorf("led: no gpio named %q", pin)
	}

	l := &LED{pin: p}

	return l, l.Set(false)
}

// Set turns the LED on or off.
func (l *LED) Set(on bool) error {
	if err := l.pin.Out(gpio.Level(on)); err != nil {
		return fmt.Errorf("led: %w", err)
	}

	return nil
}
