// Package cmd builds didban's command tree and runs it. main calls Execute.
package cmd

import (
	"context"
	"log/slog"
	"os"

	"github.com/carlmjohnson/versioninfo"
	"github.com/urfave/cli/v3"

	"github.com/1995parham-me/didban/internal/cmd/beep"
	lcdcmd "github.com/1995parham-me/didban/internal/cmd/lcd"
	"github.com/1995parham-me/didban/internal/cmd/read"
	"github.com/1995parham-me/didban/internal/cmd/rtc"
	"github.com/1995parham-me/didban/internal/cmd/serve"
)

// Execute builds the root command and runs it against os.Args, exiting
// non-zero on error.
func Execute() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	root := &cli.Command{
		Name:    "didban",
		Usage:   "home sensor-HAT monitor: a status page, Prometheus metrics and an LCD",
		Version: versioninfo.Short(),
		Commands: []*cli.Command{
			serve.Command(),
			read.Command(),
			beep.Command(),
			lcdcmd.Command(),
			rtc.Command(),
		},
	}

	if err := root.Run(context.Background(), os.Args); err != nil {
		slog.Error("didban failed", "err", err)
		os.Exit(1)
	}
}
