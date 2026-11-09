// Package buzzer drives the active-low buzzer on the HAT's PCF8574 I/O
// expander (address 0x20). The expander idles at 0xff (all pins high) =
// silent; pulling the buzzer's pin low sounds it. The driver only ever pulls
// the one configured bit, leaving any other pins on the expander untouched by
// OR-ing the rest high.
package buzzer

import (
	"fmt"
	"time"

	"periph.io/x/conn/v3/i2c"
)

// Addr is the PCF8574's I²C address on this HAT.
const Addr uint16 = 0x20

// silent is the idle port value: every pin high.
const silent = 0xff

// Buzzer is the HAT buzzer behind a PCF8574.
type Buzzer struct {
	dev *i2c.Dev
	// low is the port value that sounds the buzzer: silent with the buzzer
	// bit cleared.
	low byte
}

// New returns a buzzer whose element is on the given PCF8574 bit (0–7) and
// leaves it silent.
func New(dev *i2c.Dev, bit int) (*Buzzer, error) {
	b := &Buzzer{dev: dev, low: byte(silent) &^ (1 << bit)}
	if err := b.Silence(); err != nil {
		return nil, err
	}

	return b, nil
}

// Silence drives every pin high, stopping any sound.
func (b *Buzzer) Silence() error {
	if err := b.dev.Tx([]byte{silent}, nil); err != nil {
		return fmt.Errorf("buzzer: silence: %w", err)
	}

	return nil
}

// Beep sounds the buzzer for d, then silences it.
func (b *Buzzer) Beep(d time.Duration) error {
	if err := b.dev.Tx([]byte{b.low}, nil); err != nil {
		return fmt.Errorf("buzzer: on: %w", err)
	}

	time.Sleep(d)

	return b.Silence()
}

// Pattern sounds count beeps of length on, separated by gaps of length off.
func (b *Buzzer) Pattern(count int, on, off time.Duration) error {
	for i := range count {
		if err := b.Beep(on); err != nil {
			return err
		}

		if i < count-1 {
			time.Sleep(off)
		}
	}

	return nil
}
