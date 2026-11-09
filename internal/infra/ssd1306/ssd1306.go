// Package ssd1306 drives a 128×64 SSD1306 OLED on 4-wire SPI with separate
// data/command and reset lines, as on the Waveshare Pioneer600 (SPI0 CE0,
// DC on BCM 16, RST on BCM 19, after Waveshare's oled demo). SPI is write
// only: a missing panel cannot be detected, so New succeeds without one.
package ssd1306

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
)

// Panel size.
const (
	Width  = 128
	Height = 64
)

// initSequence sets up a 128×64 panel with the internal charge pump and
// horizontal addressing, upright on the Pioneer600 (Rotate turns it over).
var initSequence = []byte{
	0xAE,       // display off
	0xD5, 0x80, // clock divide
	0xA8, 0x3F, // multiplex 64
	0xD3, 0x00, // display offset 0
	0x40,       // start line 0
	0x8D, 0x14, // charge pump on
	0x20, 0x00, // horizontal addressing
	0xA0,       // columns left to right
	0xC0,       // rows top to bottom
	0xDA, 0x12, // COM pins
	0x81, 0xCF, // contrast
	0xD9, 0xF1, // pre-charge
	0xDB, 0x40, // VCOMH
	0xA4, // show RAM
	0xA6, // not inverted
	0xAF, // display on
}

// Display is one panel. It is not safe for concurrent use.
type Display struct {
	port spi.PortCloser
	conn spi.Conn
	dc   gpio.PinOut
	rst  gpio.PinOut
	buf  []byte
}

// New opens the SPI port (e.g. "/dev/spidev0.0") and the DC and reset GPIOs
// (periph names, e.g. "GPIO16"), resets the panel and turns it on, blank.
func New(port, dc, rst string) (*Display, error) {
	p, err := spireg.Open(port)
	if err != nil {
		return nil, fmt.Errorf("ssd1306: open %s: %w", port, err)
	}

	c, err := p.Connect(8*physic.MegaHertz, spi.Mode0, 8)
	if err != nil {
		_ = p.Close()

		return nil, fmt.Errorf("ssd1306: connect %s: %w", port, err)
	}

	d := &Display{port: p, conn: c, buf: make([]byte, Width*Height/8)}

	for name, dst := range map[string]*gpio.PinOut{dc: &d.dc, rst: &d.rst} {
		pin := gpioreg.ByName(name)
		if pin == nil {
			_ = p.Close()

			return nil, fmt.Errorf("ssd1306: no gpio named %q", name)
		}

		*dst = pin
	}

	if err := d.reset(); err != nil {
		_ = p.Close()

		return nil, err
	}

	return d, d.Show(nil)
}

func (d *Display) reset() error {
	if err := d.rst.Out(gpio.Low); err != nil {
		return fmt.Errorf("ssd1306: reset: %w", err)
	}

	time.Sleep(10 * time.Millisecond)

	if err := d.rst.Out(gpio.High); err != nil {
		return fmt.Errorf("ssd1306: reset: %w", err)
	}

	time.Sleep(10 * time.Millisecond)

	return d.command(initSequence...)
}

func (d *Display) command(b ...byte) error {
	if err := d.dc.Out(gpio.Low); err != nil {
		return fmt.Errorf("ssd1306: dc: %w", err)
	}

	if err := d.conn.Tx(b, nil); err != nil {
		return fmt.Errorf("ssd1306: command: %w", err)
	}

	return nil
}

// Show draws img (any image, a pixel lights when its gray level is above
// half) from the top-left corner; nil clears the panel.
func (d *Display) Show(img image.Image) error {
	Pack(d.buf, img)

	if err := d.command(0x21, 0, Width-1, 0x22, 0, Height/8-1); err != nil {
		return err
	}

	if err := d.dc.Out(gpio.High); err != nil {
		return fmt.Errorf("ssd1306: dc: %w", err)
	}

	// spidev takes at most 4096 bytes a transfer by default; send a page at a time.
	for i := 0; i < len(d.buf); i += Width {
		if err := d.conn.Tx(d.buf[i:i+Width], nil); err != nil {
			return fmt.Errorf("ssd1306: data: %w", err)
		}
	}

	return nil
}

// Rotate turns the image by 180°, for a panel mounted the other way up.
func (d *Display) Rotate(on bool) error {
	if on {
		return d.command(0xA1, 0xC8)
	}

	return d.command(0xA0, 0xC0)
}

// Contrast sets the brightness, 0–255.
func (d *Display) Contrast(v uint8) error { return d.command(0x81, v) }

// Close blanks the panel, turns it off and releases the port.
func (d *Display) Close() error {
	_ = d.Show(nil)
	_ = d.command(0xAE)

	if err := d.port.Close(); err != nil {
		return fmt.Errorf("ssd1306: close: %w", err)
	}

	return nil
}

// Pack turns img into the panel's memory layout: 8 pages of 128 columns, one
// byte per column with the top pixel in bit 0. nil gives a blank panel.
func Pack(buf []byte, img image.Image) {
	clear(buf)

	if img == nil {
		return
	}

	b := img.Bounds()

	for y := range Height {
		for x := range Width {
			px, py := b.Min.X+x, b.Min.Y+y
			if px >= b.Max.X || py >= b.Max.Y {
				continue
			}

			if g, ok := color.GrayModel.Convert(img.At(px, py)).(color.Gray); ok && g.Y >= 0x80 {
				buf[(y/8)*Width+x] |= 1 << (y % 8)
			}
		}
	}
}
