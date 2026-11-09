// Package rtc implements the `rtc` subcommand: show the DS3231's clock next to
// the system clock, and set it from the system clock.
package rtc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/1995parham-me/didban/internal/config"
	"github.com/1995parham-me/didban/internal/infra/station"
)

// Command returns the `rtc` subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "rtc",
		Usage: "show the DS3231 clock, or set it from the system clock",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to a TOML config file",
				Sources: cli.EnvVars("DIDBAN_CONFIG"),
			},
		},
		Commands: []*cli.Command{
			{Name: "show", Usage: "print the RTC time and its drift from the system clock", Action: show},
			{
				Name:   "sync",
				Usage:  "set the RTC from the system clock (run it only when the system clock is NTP-synced)",
				Action: sync,
			},
		},
		Action: show,
	}
}

func open(cmd *cli.Command) (*station.Station, error) {
	cfg, err := config.Load(cmd.String("config"))
	if err != nil {
		return nil, err
	}

	// Only the clock is needed.
	cfg.DS3231.Enabled = true
	cfg.BMP180.Enabled = false
	cfg.ADC.Enabled = false
	cfg.LCD.Enabled = false
	cfg.Buzzer.Enabled = false

	return station.New(cfg)
}

func show(ctx context.Context, cmd *cli.Command) error {
	st, err := open(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	r := st.Read(ctx)
	if r.RTCTime == nil {
		return errors.New("rtc: no reading from the ds3231")
	}

	out := cmd.Root().Writer
	fmt.Fprintf(out, "rtc    : %s\n", r.RTCTime.Format(time.DateTime+" -0700"))
	fmt.Fprintf(out, "system : %s\n", r.At.In(r.RTCTime.Location()).Format(time.DateTime+" -0700"))
	fmt.Fprintf(out, "drift  : %s\n", r.RTCTime.Sub(r.At).Round(time.Second))

	return nil
}

func sync(ctx context.Context, cmd *cli.Command) error {
	st, err := open(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	// The RTC keeps whole seconds; wait for the next one so the write lands
	// within a few milliseconds of the system clock.
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))

	if err := st.SetClock(time.Now()); err != nil {
		return err
	}

	return show(ctx, cmd)
}
