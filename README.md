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

didban reads the sensors on a [Waveshare Pioneer600](https://www.waveshare.com/wiki/Pioneer600) HAT over I²C and shows them on a small status page and as Prometheus metrics: room temperature, air pressure, the real-time clock, and four analog inputs. It can also sound the HAT's buzzer, keep the RTC set, and drive an HD44780 character LCD when one is connected.

It is one static binary with no CGO and no external assets, so a `GOARCH=arm64` build copies straight onto the Pi. A dead or missing chip is logged and skipped; the other sensors keep reporting.

## Hardware

| Address | Chip | What didban does with it |
| --- | --- | --- |
| `0x77` | BMP180 | temperature (°C) and barometric pressure (Pa), with the datasheet's fixed-point compensation |
| `0x68` | DS3231 | the RTC's own die temperature, and the time it holds |
| `0x48` | PCF8591 | 4 × 8-bit ADC channels, raw and scaled to volts |
| `0x20` | PCF8574 | the **active-low buzzer** on bit 7, LED2 on bit 4, and the joystick's four directions on bits 0–3 (all active-low) |
| BCM 20 | GPIO | the joystick's centre press, with a pull-up |
| 1-Wire | DS18B20 | temperature probes, through the kernel's `w1-gpio` driver (optional; the board ships one loose) |

The PCF8574 and joystick wiring comes from Waveshare's own demo code (`PCF8574/python/pcf8574.py`, `KEY/python/key.py` in the Pioneer600 code archive). Versions before the joystick support used bit 0 for the buzzer, which is the joystick's "left" line, so the buzzer stayed silent.

> [!WARNING]
> On the Pioneer600, `0x20` is the buzzer, not an LCD backpack. Writing LCD frames to it makes the buzzer chatter, so the LCD is **off by default**. If it ever beeps without stopping: `i2cset -y 1 0x20 0xff`.

Enable I²C first (`dtparam=i2c_arm=on` in `config.txt`), and check the bus with `i2cdetect -y 1`. For the DS18B20 add `dtoverlay=w1-gpio-pullup`, and for the joystick centre `gpio=20=pu`; both take effect after a reboot. Probes then appear as `/sys/bus/w1/devices/28-*`.

The BMP180 sits right above the Pi's CPU, so it reads the board's heat (around 36 °C), not the room's. When a DS18B20 probe is attached, the status page shows the probe as the temperature.

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
$ didban serve                 # poll every 15 s; status page, API and /metrics on :9465
$ didban rtc                   # the RTC's time next to the system clock
$ sudo didban rtc sync         # set the RTC from the system clock (only when NTP has set it)
$ didban lcd -s 120 dinner is ready   # show a message on the LCD for 2 min (needs an LCD; see below)
```

`didban lcd` talks to the running daemon: `serve` accepts `POST /lcd` with a JSON body `{"text": "...", "seconds": 60}` (or `"lines": [...]`, one per row), so any other service can put a message on the display. Between messages the LCD shows the latest reading.

## Status page

`serve` answers on `/` with a status page in the [Naz](https://github.com/1995parham/naz.vim) palette. It shows the current readings, the pressure trend over three hours, the RTC's drift from the system clock, 24-hour temperature and pressure charts, the joystick (each key lights while held, with its press count and the latest presses), the DS18B20 probes, the ADC channels, which chips were found, and buttons that sound the buzzer.

The joystick is polled every 50 ms. Each press is counted once however long it is held, logged, exported as a metric, and answered with a short chirp (`[joystick] beep = false` turns that off). The page is embedded in the binary and loads nothing from the internet. It refreshes itself at the poll interval.

The charts are kept in memory, one point a minute, and start empty after a restart. Prometheus keeps the long history.

The page reads a small JSON API, which other tools can use too:

| Route | |
| --- | --- |
| `GET /api/status` | host, version, uptime, the chips found, the latest reading, and `rtc_drift_seconds` |
| `GET /api/history` | the last 24 h, one sample a minute: temperature, pressure, RTC temperature |
| `GET /api/joystick` | the keys held now, presses per key, and the last ten presses |
| `POST /api/beep` | `{"count": 1}` (1–5) sounds the buzzer. It needs `Content-Type: application/json`, so a page on another site cannot trigger it, and answers 409 while a beep is still sounding |
| `POST /lcd` | a message for the LCD (see above) |
| `GET /metrics` | Prometheus |

didban has no login. Keep the port inside the house, or put a reverse proxy with an allow-list in front of it.

## Metrics

| Metric | Labels | Meaning |
| --- | --- | --- |
| `didban_up` | | 1 when the last poll read at least one sensor |
| `didban_temperature_celsius` | `sensor` (`bmp180`, `ds3231`) | temperature |
| `didban_pressure_pascals` | | barometric pressure |
| `didban_rtc_temperature_celsius` | | DS3231 die temperature |
| `didban_adc_raw` | `channel`, `label` | ADC conversion, 0–255 |
| `didban_adc_volts` | `channel`, `label` | ADC value scaled to `vref` |
| `didban_ds18b20_temperature_celsius` | `id` | each DS18B20 probe |
| `didban_joystick_presses_total` | `key` | joystick presses since start (`up`, `down`, `left`, `right`, `center`) |

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
SupplementaryGroups=i2c gpio

[Install]
WantedBy=multi-user.target
```

The `gpio` group is for the joystick centre key (`/dev/gpiochip*`). The `i2c` group must own `/dev/i2c-*` (`SUBSYSTEM=="i2c-dev", GROUP="i2c", MODE="0660"` in a udev rule).

## Development

```sh
just test    # go test -race
just lint    # golangci-lint (default: all)
just build-pi
```

The layout follows `cmd/` + `internal/{cmd,infra,sensor}`: one package per chip under `internal/infra`, `internal/infra/station` composes them, `internal/infra/web` embeds the status page, and `internal/sensor` holds the plain reading types and the chart history. The BMP180 compensation is tested against the worked example in the datasheet, and the DS3231 register encoding by a round trip.

## License

Apache-2.0
