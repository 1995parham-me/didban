// Package pcf8591 drives an NXP PCF8591 8-bit ADC/DAC over I²C (address 0x48).
// This HAT wires its four analog inputs (AIN0–AIN3) to whatever sensors the
// board carries; the driver reads all four and leaves interpretation to the
// caller, which supplies labels and a reference voltage.
package pcf8591

import (
	"fmt"

	"periph.io/x/conn/v3/i2c"
)

// Addr is the PCF8591's base I²C address on this HAT.
const Addr uint16 = 0x48

// controlAutoIncrement selects AIN0 and enables channel auto-increment, so a
// single multi-byte read walks AIN0→AIN3.
const controlAutoIncrement = 0x04

// Channels is the number of analog inputs on a PCF8591.
const Channels = 4

// ADC is a PCF8591 on a bus.
type ADC struct {
	dev *i2c.Dev
}

// New returns a PCF8591 handle.
func New(dev *i2c.Dev) *ADC {
	return &ADC{dev: dev}
}

// Read returns the four raw conversions (0–255), AIN0 first. The PCF8591
// pipelines conversions: the byte read after selecting a channel is the
// previous conversion, so the driver reads one extra leading byte and
// discards it.
func (a *ADC) Read() ([Channels]uint8, error) {
	var out [Channels]uint8

	// Write the control byte, then read Channels+1 bytes: the first is stale.
	buf := make([]byte, Channels+1)
	if err := a.dev.Tx([]byte{controlAutoIncrement}, buf); err != nil {
		return out, fmt.Errorf("pcf8591: read channels: %w", err)
	}

	copy(out[:], buf[1:])

	return out, nil
}
