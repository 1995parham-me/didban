// Package metrics turns a sensor.Reading into Prometheus gauges and serves
// them. VictoriaMetrics (or any Prometheus) scrapes /metrics; the gauges are
// updated by the poller rather than on scrape so the LCD and the exporter see
// the same values.
package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/1995parham-me/didban/internal/sensor"
)

// Collector holds the didban gauges. Construct it with New and register it on
// a prometheus.Registerer.
type Collector struct {
	up          prometheus.Gauge
	temperature *prometheus.GaugeVec
	pressure    prometheus.Gauge
	rtcTemp     prometheus.Gauge
	adcRaw      *prometheus.GaugeVec
	adcVolts    *prometheus.GaugeVec
	probes      *prometheus.GaugeVec
	presses     *prometheus.CounterVec
}

// New builds the collector. Metric names are prefixed didban_.
func New() *Collector {
	return &Collector{
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "didban_up",
			Help: "1 when the last sensor poll succeeded for at least one sensor.",
		}),
		temperature: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "didban_temperature_celsius",
			Help: "Temperature in °C, labelled by the sensor that read it.",
		}, []string{"sensor"}),
		pressure: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "didban_pressure_pascals",
			Help: "Barometric pressure in Pa from the BMP180.",
		}),
		rtcTemp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "didban_rtc_temperature_celsius",
			Help: "Die temperature in °C reported by the DS3231 RTC.",
		}),
		adcRaw: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "didban_adc_raw",
			Help: "PCF8591 ADC conversion, 0-255, per channel.",
		}, []string{"channel", "label"}),
		adcVolts: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "didban_adc_volts",
			Help: "PCF8591 ADC reading scaled to volts, per channel.",
		}, []string{"channel", "label"}),
		probes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "didban_ds18b20_temperature_celsius",
			Help: "Temperature in °C of each DS18B20 probe on the 1-Wire bus.",
		}, []string{"id"}),
		presses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "didban_joystick_presses_total",
			Help: "Joystick presses since the daemon started, per key.",
		}, []string{"key"}),
	}
}

// Press counts one joystick press.
func (c *Collector) Press(k sensor.Key) {
	c.presses.WithLabelValues(string(k)).Inc()
}

// MustRegister registers every gauge on r.
func (c *Collector) MustRegister(r prometheus.Registerer) {
	r.MustRegister(c.up, c.temperature, c.pressure, c.rtcTemp, c.adcRaw, c.adcVolts, c.probes, c.presses)

	// Every key starts at 0, so rate() and increase() work from the first press.
	for _, k := range sensor.AllKeys {
		c.presses.WithLabelValues(string(k))
	}
}

// Observe writes a reading into the gauges.
func (c *Collector) Observe(r sensor.Reading) {
	seen := false

	if r.TemperatureC != nil {
		c.temperature.WithLabelValues("bmp180").Set(*r.TemperatureC)
		seen = true
	}

	if r.PressurePa != nil {
		c.pressure.Set(*r.PressurePa)
		seen = true
	}

	if r.RTCTemperatureC != nil {
		c.rtcTemp.Set(*r.RTCTemperatureC)
		c.temperature.WithLabelValues("ds3231").Set(*r.RTCTemperatureC)
		seen = true
	}

	for _, ch := range r.ADC {
		idx := strconv.Itoa(ch.Index)
		c.adcRaw.WithLabelValues(idx, ch.Label).Set(float64(ch.Raw))
		c.adcVolts.WithLabelValues(idx, ch.Label).Set(ch.Volts)
		seen = true
	}

	c.probes.Reset() // a probe that was unplugged must not keep its last value.

	for _, p := range r.Probes {
		c.probes.WithLabelValues(p.ID).Set(p.Celsius)
		seen = true
	}

	if seen {
		c.up.Set(1)
	} else {
		c.up.Set(0)
	}
}
