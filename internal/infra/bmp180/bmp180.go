// Package bmp180 drives a Bosch BMP180 barometric pressure + temperature
// sensor over I²C (default address 0x77). periph.io's bmxx80 driver only
// covers the BMP280/BME280/BME680 family, so the older BMP180 — which uses
// a different register map and the vendor's fixed-point compensation — gets
// its own small driver here.
package bmp180

import (
	"fmt"
	"time"

	"periph.io/x/conn/v3/i2c"
)

// Addr is the BMP180's only I²C address.
const Addr uint16 = 0x77

const (
	regChipID  = 0xD0
	regCalib   = 0xAA // 22 bytes of calibration coefficients, 0xAA–0xBF.
	regControl = 0xF4
	regData    = 0xF6

	cmdReadTemp     = 0x2E
	cmdReadPressure = 0x34 // OSS=0 (ultra-low-power); higher OSS adds oss<<6.

	chipID = 0x55 // Value at regChipID that identifies a BMP180.
)

// calibration holds the eleven factory coefficients read from the chip.
type calibration struct {
	ac1, ac2, ac3      int16
	ac4, ac5, ac6      uint16
	b1, b2, mb, mc, md int16
}

// Sensor is a BMP180 on a bus.
type Sensor struct {
	dev *i2c.Dev
	cal calibration
}

// New reads the calibration table and returns a ready sensor. It fails if the
// chip-id register does not report a BMP180.
func New(dev *i2c.Dev) (*Sensor, error) {
	var id [1]byte
	if err := dev.Tx([]byte{regChipID}, id[:]); err != nil {
		return nil, fmt.Errorf("bmp180: read chip id: %w", err)
	}

	if id[0] != chipID {
		return nil, fmt.Errorf("bmp180: unexpected chip id 0x%02x (want 0x%02x)", id[0], chipID)
	}

	s := &Sensor{dev: dev}
	if err := s.readCalibration(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Sensor) readCalibration() error {
	var buf [22]byte
	if err := s.dev.Tx([]byte{regCalib}, buf[:]); err != nil {
		return fmt.Errorf("bmp180: read calibration: %w", err)
	}

	be16 := func(i int) int16 { return int16(buf[i])<<8 | int16(buf[i+1]) }
	beu16 := func(i int) uint16 { return uint16(buf[i])<<8 | uint16(buf[i+1]) }

	s.cal = calibration{
		ac1: be16(0), ac2: be16(2), ac3: be16(4),
		ac4: beu16(6), ac5: beu16(8), ac6: beu16(10),
		b1: be16(12), b2: be16(14),
		mb: be16(16), mc: be16(18), md: be16(20),
	}

	return nil
}

// Read returns the compensated temperature (°C) and pressure (Pa). The math is
// the fixed-point algorithm from the BMP180 datasheet §3.5 at OSS=0.
func (s *Sensor) Read() (float64, float64, error) {
	ut, err := s.rawTemperature()
	if err != nil {
		return 0, 0, err
	}

	up, err := s.rawPressure()
	if err != nil {
		return 0, 0, err
	}

	t, p := s.cal.compensate(ut, up)

	return t, p, nil
}

// compensate turns raw readings into temperature (°C) and pressure (Pa). It
// is the fixed-point algorithm of the BMP180 datasheet §3.5 at OSS=0, kept
// pure so it can be checked against the datasheet's worked example.
//
//nolint:gosec // the datasheet algorithm relies on these exact int32/uint32 conversions.
func (c calibration) compensate(ut uint16, up int32) (float64, float64) {
	// True temperature.
	x1 := (int32(ut) - int32(c.ac6)) * int32(c.ac5) >> 15
	x2 := int32(c.mc) << 11 / (x1 + int32(c.md))
	b5 := x1 + x2
	t := float64((b5+8)>>4) / 10.0 // 0.1 °C units.

	// True pressure (OSS=0).
	b6 := b5 - 4000
	x1 = (int32(c.b2) * (b6 * b6 >> 12)) >> 11
	x2 = int32(c.ac2) * b6 >> 11
	x3 := x1 + x2
	b3 := (((int32(c.ac1)*4 + x3) << 0) + 2) / 4
	x1 = int32(c.ac3) * b6 >> 13
	x2 = (int32(c.b1) * (b6 * b6 >> 12)) >> 16
	x3 = ((x1 + x2) + 2) >> 2
	b4 := uint32(c.ac4) * uint32(x3+32768) >> 15
	b7 := (uint32(up) - uint32(b3)) * 50000
	var p int32
	if b7 < 0x80000000 {
		p = int32(b7 * 2 / b4)
	} else {
		p = int32(b7 / b4 * 2)
	}
	x1 = (p >> 8) * (p >> 8)
	x1 = (x1 * 3038) >> 16
	x2 = (-7357 * p) >> 16
	p += (x1 + x2 + 3791) >> 4

	return t, float64(p)
}

func (s *Sensor) rawTemperature() (uint16, error) {
	if err := s.dev.Tx([]byte{regControl, cmdReadTemp}, nil); err != nil {
		return 0, fmt.Errorf("bmp180: start temperature: %w", err)
	}

	time.Sleep(5 * time.Millisecond)

	var buf [2]byte
	if err := s.dev.Tx([]byte{regData}, buf[:]); err != nil {
		return 0, fmt.Errorf("bmp180: read temperature: %w", err)
	}

	return uint16(buf[0])<<8 | uint16(buf[1]), nil
}

func (s *Sensor) rawPressure() (int32, error) {
	if err := s.dev.Tx([]byte{regControl, cmdReadPressure}, nil); err != nil {
		return 0, fmt.Errorf("bmp180: start pressure: %w", err)
	}

	time.Sleep(5 * time.Millisecond)

	var buf [3]byte
	if err := s.dev.Tx([]byte{regData}, buf[:]); err != nil {
		return 0, fmt.Errorf("bmp180: read pressure: %w", err)
	}

	// 19-bit result, right-shifted by (8 - OSS); OSS=0.
	return (int32(buf[0])<<16 | int32(buf[1])<<8 | int32(buf[2])) >> 8, nil
}
