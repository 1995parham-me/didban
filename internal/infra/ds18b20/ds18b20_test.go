package ds18b20

import (
	"os"
	"path/filepath"
	"testing"
)

func probe(t *testing.T, dir, name, file, body string) {
	t.Helper()

	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(p, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadProbes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	probe(t, dir, "28-0000000000b2", "temperature", "21500\n")
	probe(t, dir, "28-0000000000a1", "w1_slave",
		"72 01 4b 46 7f ff 0e 10 57 : crc=57 YES\n72 01 4b 46 7f ff 0e 10 57 t=23125\n")
	probe(t, dir, "28-0000000000c3", "w1_slave",
		"72 01 4b 46 7f ff 0e 10 57 : crc=00 NO\n72 01 4b 46 7f ff 0e 10 57 t=85000\n")
	probe(t, dir, "w1_bus_master1", "temperature", "1\n") // not a probe

	probes, err := Read(dir)
	if err == nil {
		t.Error("want an error for the probe with a bad CRC")
	}

	if len(probes) != 2 {
		t.Fatalf("probes = %v, want 2", probes)
	}

	if probes[0].ID != "28-0000000000a1" || probes[0].Celsius != 23.125 {
		t.Errorf("first = %+v", probes[0])
	}

	if probes[1].ID != "28-0000000000b2" || probes[1].Celsius != 21.5 {
		t.Errorf("second = %+v", probes[1])
	}
}

func TestReadNoProbes(t *testing.T) {
	t.Parallel()

	probes, err := Read(t.TempDir())
	if err != nil || len(probes) != 0 {
		t.Errorf("Read(empty) = %v, %v", probes, err)
	}
}

func TestNegative(t *testing.T) {
	t.Parallel()

	if c, err := parseSlave("x : crc=1 YES\nx t=-10125\n"); err != nil || c != -10.125 {
		t.Errorf("parseSlave = %v, %v", c, err)
	}
}
