// Package lcd drives an HD44780 character LCD through a PCF8574 I²C backpack
// (the common "LCD1602 I²C" board, address 0x20 or 0x27) in 4-bit mode. The
// backpack's standard pin map is P0=RS, P1=RW, P2=EN, P3=backlight,
// P4–P7=D4–D7.
package lcd

import (
	"fmt"
	"time"

	"periph.io/x/conn/v3/i2c"
)

// Backpack pin bits on the PCF8574.
const (
	pinRS        = 0x01
	pinEnable    = 0x04
	pinBacklight = 0x08
)

// mode flags for a write: command (RS low) or data (RS high).
const (
	modeCommand = 0x00
	modeData    = pinRS
)

// HD44780 commands used here.
const (
	cmdClear       = 0x01
	cmdEntryMode   = 0x06 // increment cursor, no shift.
	cmdDisplayOn   = 0x0C // display on, cursor off, blink off.
	cmdFunctionSet = 0x28 // 4-bit, 2 lines, 5x8 font.
	cmdSetDDRAM    = 0x80
)

// Display is an HD44780 LCD behind a PCF8574 on a bus.
type Display struct {
	dev       *i2c.Dev
	cols      int
	rows      int
	backlight byte
}

// New returns an LCD handle for a cols×rows panel. Call Init before writing.
func New(dev *i2c.Dev, cols, rows int) *Display {
	return &Display{dev: dev, cols: cols, rows: rows, backlight: pinBacklight}
}

// Init runs the HD44780 4-bit initialization sequence.
func (d *Display) Init() error {
	time.Sleep(50 * time.Millisecond)

	// Three 0x3 nibbles wake the controller, then 0x2 switches it to 4-bit.
	for _, delay := range []time.Duration{5 * time.Millisecond, 150 * time.Microsecond, 150 * time.Microsecond} {
		if err := d.writeNibble(0x30, modeCommand); err != nil {
			return err
		}

		time.Sleep(delay)
	}

	if err := d.writeNibble(0x20, modeCommand); err != nil {
		return err
	}

	for _, c := range []byte{cmdFunctionSet, cmdDisplayOn, cmdClear, cmdEntryMode} {
		if err := d.command(c); err != nil {
			return err
		}
	}

	time.Sleep(2 * time.Millisecond)

	return nil
}

// Backlight turns the backlight on or off; it takes effect on the next write.
func (d *Display) Backlight(on bool) {
	if on {
		d.backlight = pinBacklight
	} else {
		d.backlight = 0
	}
}

// Clear blanks the display and homes the cursor.
func (d *Display) Clear() error {
	if err := d.command(cmdClear); err != nil {
		return err
	}

	time.Sleep(2 * time.Millisecond)

	return nil
}

// Lines writes each string to its own row, truncated or space-padded to the
// panel width, starting at row 0. Extra lines beyond the panel are ignored.
func (d *Display) Lines(lines []string) error {
	// DDRAM start address of each row on a 1- or 2-line HD44780 panel.
	rowAddr := []byte{0x00, 0x40, byte(0x00 + d.cols), byte(0x40 + d.cols)} //nolint:gosec // cols is 16 or 20.

	for row := range d.rows {
		if err := d.command(cmdSetDDRAM | rowAddr[row]); err != nil {
			return err
		}

		text := ""
		if row < len(lines) {
			text = lines[row]
		}

		for _, r := range fit(text, d.cols) {
			if err := d.writeByte(byte(r), modeData); err != nil { //nolint:gosec // fit() returns ASCII only.
				return err
			}
		}
	}

	return nil
}

// command writes an instruction byte.
func (d *Display) command(b byte) error {
	return d.writeByte(b, modeCommand)
}

// writeByte sends a full byte as two 4-bit nibbles, high first.
func (d *Display) writeByte(value, mode byte) error {
	if err := d.writeNibble(value&0xf0, mode); err != nil {
		return err
	}

	return d.writeNibble((value<<4)&0xf0, mode)
}

// writeNibble places the high nibble on D4–D7 and strobes the enable line.
func (d *Display) writeNibble(nibble, mode byte) error {
	b := nibble | mode | d.backlight

	if err := d.dev.Tx([]byte{b | pinEnable}, nil); err != nil {
		return fmt.Errorf("lcd: strobe high: %w", err)
	}

	time.Sleep(1 * time.Microsecond)

	if err := d.dev.Tx([]byte{b &^ pinEnable}, nil); err != nil {
		return fmt.Errorf("lcd: strobe low: %w", err)
	}

	time.Sleep(50 * time.Microsecond)

	return nil
}

// fit truncates or right-pads s with spaces to exactly width runes.
func fit(s string, width int) []rune {
	r := []rune(s)
	if len(r) > width {
		return r[:width]
	}

	for len(r) < width {
		r = append(r, ' ')
	}

	return r
}
