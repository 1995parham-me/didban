// Package ds18b20 reads DS18B20 temperature probes through the Linux 1-Wire
// driver (w1-gpio + w1-therm, `dtoverlay=w1-gpio-pullup` in config.txt on a
// Pi). The kernel does the bus timing, so this is a sysfs reader: every probe
// is a directory `28-<serial>` under /sys/bus/w1/devices. Reading a probe
// starts a conversion and takes up to ~750 ms.
package ds18b20

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/1995parham-me/didban/internal/sensor"
)

// Dir is where the kernel lists 1-Wire devices.
const Dir = "/sys/bus/w1/devices"

// family is the DS18B20's 1-Wire family code, the prefix of its device name.
const family = "28-"

// errNotReady is a failed CRC or an unfinished conversion in w1_slave.
var errNotReady = errors.New("ds18b20: reading not ready (crc)")

// Read returns every probe under dir, in serial order. A probe that fails is
// skipped and its error joined into the returned error, so one bad probe does
// not hide the others. No probes is not an error.
func Read(dir string) ([]sensor.Probe, error) {
	matches, err := filepath.Glob(filepath.Join(dir, family+"*"))
	if err != nil {
		return nil, fmt.Errorf("ds18b20: list probes: %w", err)
	}

	slices.Sort(matches)

	var (
		probes []sensor.Probe
		errs   []error
	)

	for _, m := range matches {
		c, err := readProbe(m)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		probes = append(probes, sensor.Probe{ID: filepath.Base(m), Celsius: c})
	}

	return probes, errors.Join(errs...)
}

// readProbe reads one probe directory: `temperature` (millidegrees) on
// current kernels, else the older `w1_slave` with its CRC line.
func readProbe(path string) (float64, error) {
	// path comes from a glob of the configured sysfs directory.
	if b, err := os.ReadFile(filepath.Join(path, "temperature")); err == nil { //nolint:gosec // see above.
		return milli(strings.TrimSpace(string(b)))
	}

	b, err := os.ReadFile(filepath.Join(path, "w1_slave")) //nolint:gosec // a sysfs path from the glob.
	if err != nil {
		return 0, fmt.Errorf("ds18b20: read %s: %w", filepath.Base(path), err)
	}

	return parseSlave(string(b))
}

// parseSlave parses w1_slave: a first line ending in YES when the CRC
// matched, and a second line ending in t=<millidegrees>.
func parseSlave(s string) (float64, error) {
	first, rest, _ := strings.Cut(s, "\n")
	if !strings.HasSuffix(strings.TrimSpace(first), "YES") {
		return 0, errNotReady
	}

	_, t, ok := strings.Cut(rest, "t=")
	if !ok {
		return 0, fmt.Errorf("ds18b20: no t= in %q", rest)
	}

	return milli(strings.TrimSpace(t))
}

func milli(s string) (float64, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("ds18b20: parse %q: %w", s, err)
	}

	return float64(v) / 1000, nil
}
