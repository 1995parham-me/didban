// Package beep implements the `beep` subcommand: sound the HAT buzzer, for
// testing and for finding which PCF8574 bit the buzzer element sits on.
package beep

import (
	"context"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/1995parham-me/didban/internal/config"
	"github.com/1995parham-me/didban/internal/infra/station"
)

// Command returns the `beep` subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "beep",
		Usage: "sound the buzzer (count beeps of ms each)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to a TOML config file",
				Sources: cli.EnvVars("DIDBAN_CONFIG"),
			},
			&cli.IntFlag{Name: "bit", Usage: "override the buzzer PCF8574 bit (0-7)", Value: -1},
			&cli.IntFlag{Name: "ms", Usage: "beep length in milliseconds", Value: 120},
			&cli.IntFlag{Name: "count", Usage: "number of beeps", Value: 1},
		},
		Action: run,
	}
}

func run(_ context.Context, cmd *cli.Command) error {
	cfg, err := config.Load(cmd.String("config"))
	if err != nil {
		return err
	}

	cfg.Buzzer.Enabled = true
	if bit := cmd.Int("bit"); bit >= 0 {
		cfg.Buzzer.Bit = bit
	}

	// Only the buzzer is needed; skip the sensors and the LCD entirely.
	cfg.BMP180.Enabled = false
	cfg.DS3231.Enabled = false
	cfg.ADC.Enabled = false
	cfg.LCD.Enabled = false

	st, err := station.New(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	on := time.Duration(cmd.Int("ms")) * time.Millisecond

	count := cmd.Int("count")

	for i := range count {
		if bErr := st.Beep(on); bErr != nil {
			return bErr
		}

		if i < count-1 {
			time.Sleep(on)
		}
	}

	return nil
}
