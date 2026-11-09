// Package config loads didban's configuration, layering hardcoded defaults,
// an optional TOML file, and DIDBAN_-prefixed environment variables through
// koanf. The defaults alone are enough to run on the reference Waveshare-style
// HAT (BMP180 + DS3231 + PCF8591 + PCF8574 LCD).
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// Config is the whole configuration tree.
type Config struct {
	I2C    I2C    `json:"i2c"    koanf:"i2c"`
	Poll   Poll   `json:"poll"   koanf:"poll"`
	Server Server `json:"server" koanf:"server"`
	LCD    LCD    `json:"lcd"    koanf:"lcd"`
	Buzzer Buzzer `json:"buzzer" koanf:"buzzer"`
	BMP180 Device `json:"bmp180" koanf:"bmp180"`
	DS3231 RTC    `json:"ds3231" koanf:"ds3231"`
	ADC    ADC    `json:"adc"    koanf:"adc"`

	Joystick Joystick `json:"joystick" koanf:"joystick"`
	DS18B20  OneWire  `json:"ds18b20"  koanf:"ds18b20"`
	Tunnel   Tunnel   `json:"tunnel"   koanf:"tunnel"`
}

// Tunnel is a sing-box tunnel on the same host, read through its Clash API:
// the exit Selector uses, the probe delays, the throughput and the open
// flows. Secret is the API's bearer token.
type Tunnel struct {
	Enabled  bool          `json:"enabled"  koanf:"enabled"`
	API      string        `json:"api"      koanf:"api"`
	Secret   string        `json:"-"        koanf:"secret"`
	Selector string        `json:"selector" koanf:"selector"`
	Poll     time.Duration `json:"poll"     koanf:"poll"`
}

// Joystick is the five-way switch: four directions on the PCF8574 at Address
// (bits 0–3), the centre on the GPIO named Center. Beep chirps the buzzer on
// every press.
type Joystick struct {
	Enabled bool          `json:"enabled" koanf:"enabled"`
	Address uint16        `json:"address" koanf:"address"`
	Center  string        `json:"center"  koanf:"center"`
	Poll    time.Duration `json:"poll"    koanf:"poll"`
	Beep    bool          `json:"beep"    koanf:"beep"`
}

// OneWire is where the kernel lists DS18B20 probes.
type OneWire struct {
	Enabled bool   `json:"enabled" koanf:"enabled"`
	Dir     string `json:"dir"     koanf:"dir"`
}

// Buzzer describes the active-low buzzer on a PCF8574 I/O expander: the
// expander address and which of its bits (0–7) the buzzer element sits on.
type Buzzer struct {
	Enabled bool   `json:"enabled" koanf:"enabled"`
	Address uint16 `json:"address" koanf:"address"`
	Bit     int    `json:"bit"     koanf:"bit"`
}

// I2C selects the bus.
type I2C struct {
	Bus string `json:"bus" koanf:"bus"`
}

// Poll controls the read loop.
type Poll struct {
	Interval time.Duration `json:"interval" koanf:"interval"`
}

// Server is the HTTP endpoint: the status page, /api/, /metrics and POST /lcd.
type Server struct {
	Address string `json:"address" koanf:"address"`
}

// Device is a generic enable+address for a single-address sensor.
type Device struct {
	Enabled bool   `json:"enabled" koanf:"enabled"`
	Address uint16 `json:"address" koanf:"address"`
}

// RTC adds a timezone to a Device for interpreting the kept clock.
type RTC struct {
	Enabled  bool   `json:"enabled"  koanf:"enabled"`
	Address  uint16 `json:"address"  koanf:"address"`
	Timezone string `json:"timezone" koanf:"timezone"`
}

// LCD describes the character display.
type LCD struct {
	Enabled   bool   `json:"enabled"   koanf:"enabled"`
	Address   uint16 `json:"address"   koanf:"address"`
	Cols      int    `json:"cols"      koanf:"cols"`
	Rows      int    `json:"rows"      koanf:"rows"`
	Backlight bool   `json:"backlight" koanf:"backlight"`
}

// ADC describes the PCF8591 and how to interpret its channels.
type ADC struct {
	Enabled bool     `json:"enabled" koanf:"enabled"`
	Address uint16   `json:"address" koanf:"address"`
	VRef    float64  `json:"vref"    koanf:"vref"`
	Labels  []string `json:"labels"  koanf:"labels"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		I2C:    I2C{Bus: "1"},
		Poll:   Poll{Interval: 15 * time.Second},
		Server: Server{Address: ":9465"},
		// The LCD is off by default: on the reference HAT 0x20 is the buzzer,
		// not an LCD. Enable it only once a real display is located, at its own
		// address.
		LCD: LCD{Enabled: false, Address: 0x27, Cols: 16, Rows: 2, Backlight: true},
		// The buzzer is PCF8574 bit 7 (Waveshare's beep_on clears 0x80);
		// bits 0–3 are the joystick and bit 4 is LED2.
		Buzzer: Buzzer{Enabled: true, Address: 0x20, Bit: 7},
		BMP180: Device{Enabled: true, Address: 0x77},
		DS3231: RTC{Enabled: true, Address: 0x68, Timezone: "Asia/Tehran"},
		ADC: ADC{
			Enabled: true, Address: 0x48, VRef: 3.3,
			Labels: []string{"ain0", "ain1", "ain2", "ain3"},
		},
		Joystick: Joystick{Enabled: true, Address: 0x20, Center: "GPIO20", Poll: 50 * time.Millisecond, Beep: true},
		DS18B20:  OneWire{Enabled: true, Dir: "/sys/bus/w1/devices"},
		Tunnel:   Tunnel{Enabled: false, API: "http://127.0.0.1:9099", Selector: "tunnel", Poll: 2 * time.Second},
	}
}

// Location resolves the DS3231 timezone, falling back to UTC.
func (c Config) Location() *time.Location {
	loc, err := time.LoadLocation(c.DS3231.Timezone)
	if err != nil {
		return time.UTC
	}

	return loc
}

// Load builds the configuration: defaults, then the file at path when it is
// non-empty, then DIDBAN_-prefixed environment variables (double underscore
// nests, e.g. DIDBAN_POLL__INTERVAL).
func Load(path string) (Config, error) {
	k := koanf.New(".")

	if err := k.Load(structs.Provider(Default(), "koanf"), nil); err != nil {
		return Config{}, fmt.Errorf("load defaults: %w", err)
	}

	if path != "" {
		if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
			return Config{}, fmt.Errorf("load config file %q: %w", path, err)
		}
	}

	envProvider := env.Provider(".", env.Opt{
		Prefix: "DIDBAN_",
		TransformFunc: func(key, val string) (string, any) {
			key = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(key, "DIDBAN_")), "__", ".")

			return key, val
		},
	})
	if err := k.Load(envProvider, nil); err != nil {
		return Config{}, fmt.Errorf("load env: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, nil
}
