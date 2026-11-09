// Package read implements the `read` subcommand: a one-shot poll of every
// sensor printed to stdout, for checking the HAT without the daemon.
package read

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/1995parham-me/didban/internal/config"
	"github.com/1995parham-me/didban/internal/infra/station"
)

// Command returns the `read` subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "read",
		Usage: "poll every sensor once and print the values",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to a TOML config file",
				Sources: cli.EnvVars("DIDBAN_CONFIG"),
			},
			&cli.BoolFlag{Name: "json", Usage: "emit the reading as JSON"},
		},
		Action: run,
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	cfg, err := config.Load(cmd.String("config"))
	if err != nil {
		return err
	}

	st, err := station.New(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	r := st.Read(ctx)
	out := cmd.Root().Writer

	if cmd.Bool("json") {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")

		return enc.Encode(r) //nolint:wrapcheck // CLI leaf; root logs the error.
	}

	if r.RTCTime != nil {
		fmt.Fprintf(out, "rtc time    : %s\n", r.RTCTime.Format("2006-01-02 15:04:05 MST"))
	}

	if r.TemperatureC != nil {
		fmt.Fprintf(out, "temperature : %.2f °C (bmp180)\n", *r.TemperatureC)
	}

	if r.RTCTemperatureC != nil {
		fmt.Fprintf(out, "rtc temp    : %.2f °C (ds3231)\n", *r.RTCTemperatureC)
	}

	if r.PressurePa != nil {
		fmt.Fprintf(out, "pressure    : %.1f hPa\n", *r.PressurePa/100.0)
	}

	for _, ch := range r.ADC {
		fmt.Fprintf(out, "adc[%d] %-6s: raw=%3d  %.3f V\n", ch.Index, ch.Label, ch.Raw, ch.Volts)
	}

	for _, p := range r.Probes {
		fmt.Fprintf(out, "probe       : %.3f °C (ds18b20 %s)\n", p.Celsius, p.ID)
	}

	return nil
}
