// Package serve runs the didban daemon: it polls the sensor station on an
// interval, updates the Prometheus gauges, renders a status screen on the LCD,
// and serves the house-status page at / (with its JSON API under /api/),
// /metrics, and a POST /lcd endpoint for pushing custom messages.
package serve

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/urfave/cli/v3"

	"github.com/1995parham-me/didban/internal/config"
	"github.com/1995parham-me/didban/internal/infra/metrics"
	"github.com/1995parham-me/didban/internal/infra/station"
)

// Command returns the `serve` subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "poll the sensors, export Prometheus metrics, and drive the LCD",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to a TOML config file",
				Sources: cli.EnvVars("DIDBAN_CONFIG"),
			},
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
	defer func() {
		if cErr := st.Close(); cErr != nil {
			slog.Error("closing station", "err", cErr)
		}
	}()

	reg := prometheus.NewRegistry()
	coll := metrics.New()
	coll.MustRegister(reg)

	msg := &message{}
	web := newServer(cfg, reg, st, msg)

	srv := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           web.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("serving", "address", cfg.Server.Address)
		if sErr := srv.ListenAndServe(); sErr != nil && !errors.Is(sErr, http.ErrServerClosed) {
			slog.Error("http server stopped", "err", sErr)
		}
	}()

	poll(ctx, cfg, st, coll, msg, web)

	shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	return srv.Shutdown(shutCtx) //nolint:wrapcheck // returned straight to the CLI root, which logs it.
}

// poll reads the station every interval until ctx is done, updating metrics and
// the LCD and the web page on each tick.
func poll(ctx context.Context, cfg config.Config, st *station.Station, coll *metrics.Collector, msg *message, web *server) {
	ticker := time.NewTicker(cfg.Poll.Interval)
	defer ticker.Stop()

	render := func() {
		reading := st.Read(ctx)
		coll.Observe(reading)
		web.observe(reading)

		if !st.HasLCD() {
			return
		}

		lines := msg.active(time.Now())
		if lines == nil {
			lines = statusLines(reading, cfg.LCD.Cols)
		}

		if err := st.ShowLines(lines); err != nil {
			slog.WarnContext(ctx, "lcd update failed", "err", err)
		}
	}

	render() // show something immediately rather than after one interval.

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			render()
		}
	}
}
