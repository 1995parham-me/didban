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
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/urfave/cli/v3"

	"github.com/1995parham-me/didban/internal/config"
	"github.com/1995parham-me/didban/internal/infra/clashapi"
	"github.com/1995parham-me/didban/internal/infra/led"
	"github.com/1995parham-me/didban/internal/infra/metrics"
	"github.com/1995parham-me/didban/internal/infra/ssd1306"
	"github.com/1995parham-me/didban/internal/infra/station"
	"github.com/1995parham-me/didban/internal/tunnel"
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
	web := newServer(cfg, reg, coll, st, msg)

	if cfg.Tunnel.Enabled {
		web.tunnel = tunnel.New(clashapi.New(cfg.Tunnel.API, cfg.Tunnel.Secret), cfg.Tunnel.Selector, historySize)
		go web.tunnel.Run(ctx, cfg.Tunnel.Poll)
	}

	done := startOutputs(ctx, cfg, web)
	defer done()

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

	go web.watchJoystick(ctx)

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

// startOutputs starts the OLED and the LED when they are enabled and work.
// The returned function waits for both to blank themselves after ctx ends.
func startOutputs(ctx context.Context, cfg config.Config, web *server) func() {
	var wg sync.WaitGroup

	if cfg.OLED.Enabled {
		if d, err := ssd1306.New(cfg.OLED.SPI, cfg.OLED.DC, cfg.OLED.Reset); err != nil {
			slog.Warn("oled unavailable", "err", err)
		} else {
			if err := d.Contrast(cfg.OLED.Contrast); err != nil {
				slog.Warn("oled contrast", "err", err)
			}

			if err := d.Rotate(cfg.OLED.Rotate); err != nil {
				slog.Warn("oled rotate", "err", err)
			}

			web.oled = true

			wg.Go(func() { web.drawOLED(ctx, d) })
		}
	}

	if cfg.LED.Enabled {
		if l, err := led.New(cfg.LED.Pin); err != nil {
			slog.Warn("led unavailable", "err", err)
		} else {
			web.ledState.Store(patternOff)

			wg.Go(func() { web.driveLED(ctx, l) })
		}
	}

	return wg.Wait
}
