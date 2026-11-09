// Package i2cbus opens the Raspberry Pi I²C bus through periph.io and hands
// out per-address device handles. It is the one place that touches periph's
// host registration, so the drivers above it stay free of that wiring.
package i2cbus

import (
	"fmt"

	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

// Open opens the named I²C bus. name may be a number ("1"), a device path
// ("/dev/i2c-1"), or "" for the first bus periph finds. The caller closes the
// returned bus.
func Open(name string) (i2c.BusCloser, error) { //nolint:ireturn // periph only exposes the bus as an interface.
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("periph host init: %w", err)
	}

	bus, err := i2creg.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open i2c bus %q: %w", name, err)
	}

	return bus, nil
}

// Dev wraps an address on a bus as an addressable device.
func Dev(bus i2c.Bus, addr uint16) *i2c.Dev {
	return &i2c.Dev{Bus: bus, Addr: addr}
}
