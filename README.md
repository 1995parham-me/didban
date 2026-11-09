<h1 align="center">didban</h1>

<p align="center">
  <em>دیدبان — "the lookout". A small Go daemon that watches the house through a Raspberry Pi sensor HAT and reports to Prometheus.</em>
</p>

<p align="center">
  <a href="https://github.com/1995parham-me/didban/actions/workflows/test.yaml"><img alt="test" src="https://img.shields.io/github/actions/workflow/status/1995parham-me/didban/test.yaml?branch=main&style=flat-square&label=test"></a>
  <a href="https://github.com/1995parham-me/didban/releases"><img alt="release" src="https://img.shields.io/github/v/release/1995parham-me/didban?style=flat-square"></a>
  <img alt="go" src="https://img.shields.io/github/go-mod/go-version/1995parham-me/didban?style=flat-square">
  <img alt="license" src="https://img.shields.io/github/license/1995parham-me/didban?style=flat-square">
</p>

---

didban reads the sensors on a [Waveshare Pioneer600](https://www.waveshare.com/wiki/Pioneer600) HAT over I²C and exposes them as Prometheus metrics: room temperature, air pressure, the real-time clock, and four analog inputs. It can also sound the HAT's buzzer, and drive an HD44780 character LCD when one is connected.

It is one static binary with no CGO, so a `GOARCH=arm64` build copies straight onto the Pi. A dead or missing chip is logged and skipped; the other sensors keep reporting.

## Hardware

| Address | Chip | What didban does with it |
| --- | --- | --- |
| `0x77` | BMP180 | temperature (°C) and barometric pressure (Pa), with the datasheet's fixed-point compensation |
| `0x68` | DS3231 | the RTC's own die temperature, and the time it holds |
| `0x48` | PCF8591 | 4 × 8-bit ADC channels, raw and scaled to volts |
| `0x20` | PCF8574 | the **active-low buzzer**: `0xff` is silent, clearing one bit sounds it |

> [!WARNING]
> On the Pioneer600, `0x20` is the buzzer, not an LCD backpack. Writing LCD frames to it makes the buzzer chatter, so the LCD is **off by default**. If it ever beeps without stopping: `i2cset -y 1 0x20 0xff`.

Enable I²C first (`dtparam=i2c_arm=on` in `config.txt`), and check the bus with `i2cdetect -y 1`.

## Install

Download a binary from [Releases](https://github.com/1995parham-me/didban/releases) (`linux_arm64` for a Pi 4/5, `linux_armv7`/`armv6` for older boards), or build it:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o didban ./cmd/didban
scp didban pi:/usr/local/bin/
```

## Usage

```console
$ didban read
rtc time    : 2026-11-09 04:12:51 +0330
temperature : 24.80 °C (bmp180)
rtc temp    : 25.25 °C (ds3231)
pressure    : 880.4 hPa
adc[0] ain0  : raw=212  2.744 V
...

$ didban read --json          # the same reading as JSON
$ didban beep --count 3        # three short beeps
$ didban serve                 # poll every 15 s, serve /metrics on :9465
$ didban lcd -s 120 dinner is ready   # show a message on the LCD for 2 min (needs an LCD; see below)
```

`didban lcd` talks to the running daemon: `serve` accepts `POST /lcd` with a JSON body `{"text": "...", "seconds": 60}` (or `"lines": [...]`, one per row), so any other service can put a message on the display. Between messages the LCD shows the latest reading.

## Metrics

| Metric | Labels | Meaning |
| --- | --- | --- |
| `didban_up` | | 1 when the last poll read at least one sensor |
| `didban_temperature_celsius` | `sensor` (`bmp180`, `ds3231`) | temperature |
| `didban_pressure_pascals` | | barometric pressure |
| `didban_rtc_temperature_celsius` | | DS3231 die temperature |
| `didban_adc_raw` | `channel`, `label` | ADC conversion, 0–255 |
| `didban_adc_volts` | `channel`, `label` | ADC value scaled to `vref` |

Scrape it like any exporter:

```yaml
scrape_configs:
  - job_name: didban
    static_configs:
      - targets: ["chopper.lan:9465"]
```

A few useful queries: `didban_temperature_celsius{sensor="bmp180"}` for the room, `didban_pressure_pascals / 100` for hPa, `deriv(didban_pressure_pascals[3h])` for a falling barometer.

## Configuration

Every setting has a default that matches the Pioneer600, so no file is needed. To change something, pass `--config` (or `DIDBAN_CONFIG`) with a TOML file; [`configs/config.example.toml`](configs/config.example.toml) lists every key. Environment variables win over the file, with `__` for nesting:

```sh
DIDBAN_POLL__INTERVAL=5s DIDBAN_BUZZER__ENABLED=false didban serve
```

To use a real HD44780 LCD on a PCF8574 backpack, set `[lcd] enabled = true` and its own `address` (usually `0x27` or `0x3f`), never the buzzer's.

## Run as a service

```ini
[Unit]
Description=didban
After=network-online.target

[Service]
ExecStart=/usr/local/bin/didban serve
Restart=always
DynamicUser=yes
SupplementaryGroups=i2c

[Install]
WantedBy=multi-user.target
```

The `i2c` group must own `/dev/i2c-*` (`SUBSYSTEM=="i2c-dev", GROUP="i2c", MODE="0660"` in a udev rule).

## Development

```sh
just test    # go test -race
just lint    # golangci-lint (default: all)
just build-pi
```

The layout follows `cmd/` + `internal/{cmd,infra,sensor}`: one package per chip under `internal/infra`, `internal/infra/station` composes them, and `internal/sensor` holds the plain reading types. The BMP180 compensation is tested against the worked example in the datasheet.

## License

Apache-2.0
