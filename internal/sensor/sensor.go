// Package sensor holds the pure value types the drivers produce and the
// daemon consumes. It imports nothing else in the project and talks to no
// hardware — that keeps it unit-testable and the dependency arrows pointing
// inward (domain ← infra ← cmd).
package sensor

import "time"

// Reading is a single poll of every sensor on the HAT. Pointer fields are nil
// when that sensor is disabled or its read failed, so a consumer can tell
// "not measured" from a real zero.
type Reading struct {
	// At is when the poll was taken.
	At time.Time `json:"at"`

	// BMP180 environment sensor (0x77).
	TemperatureC *float64 `json:"temperature_c,omitempty"`
	PressurePa   *float64 `json:"pressure_pa,omitempty"`

	// DS3231 RTC (0x68): the die temperature it reports, and the wall-clock
	// time it is holding.
	RTCTemperatureC *float64   `json:"rtc_temperature_c,omitempty"`
	RTCTime         *time.Time `json:"rtc_time,omitempty"`

	// PCF8591 ADC (0x48): the four analog channels, raw 0–255 and scaled to
	// volts against the configured reference.
	ADC []ADCChannel `json:"adc,omitempty"`
}

// ADCChannel is one PCF8591 analog input.
type ADCChannel struct {
	// Index is the channel number, 0–3.
	Index int `json:"index"`
	// Label is a human name for whatever is wired to it (config-supplied).
	Label string `json:"label"`
	// Raw is the 8-bit conversion, 0–255.
	Raw uint8 `json:"raw"`
	// Volts is Raw scaled to the configured reference voltage.
	Volts float64 `json:"volts"`
}
