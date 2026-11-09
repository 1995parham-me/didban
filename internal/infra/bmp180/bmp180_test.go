package bmp180

import "testing"

// TestCompensateDatasheet checks the worked example of the BMP180 datasheet
// (§3.5): these coefficients and raw values give 15.0 °C and 69964 Pa.
func TestCompensateDatasheet(t *testing.T) {
	t.Parallel()

	c := calibration{
		ac1: 408, ac2: -72, ac3: -14383,
		ac4: 32741, ac5: 32757, ac6: 23153,
		b1: 6190, b2: 4,
		mb: -32768, mc: -8711, md: 2868,
	}

	temp, pres := c.compensate(27898, 23843)

	if temp != 15.0 {
		t.Errorf("temperature = %v, want 15.0", temp)
	}

	if pres != 69964 {
		t.Errorf("pressure = %v, want 69964", pres)
	}
}
