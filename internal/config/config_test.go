package config_test

import (
	"testing"
	"time"

	"github.com/1995parham-me/didban/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.LCD.Enabled {
		t.Error("LCD must be off by default: 0x20 is the buzzer on the Pioneer600")
	}

	if cfg.Buzzer.Address != 0x20 || cfg.BMP180.Address != 0x77 || cfg.DS3231.Address != 0x68 || cfg.ADC.Address != 0x48 {
		t.Errorf("unexpected default addresses: %+v", cfg)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("DIDBAN_POLL__INTERVAL", "5s")
	t.Setenv("DIDBAN_SERVER__ADDRESS", ":9999")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Poll.Interval != 5*time.Second {
		t.Errorf("poll interval = %v, want 5s", cfg.Poll.Interval)
	}

	if cfg.Server.Address != ":9999" {
		t.Errorf("server address = %q, want :9999", cfg.Server.Address)
	}
}
