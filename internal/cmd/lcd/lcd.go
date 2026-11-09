// Package lcd implements the `lcd` subcommand: it pushes a message to the
// running daemon's POST /lcd endpoint so the text appears on the display.
package lcd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// Command returns the `lcd` subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "lcd",
		Usage:     "show a message on the LCD via the running daemon",
		ArgsUsage: "<message words...>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "url",
				Usage:   "base URL of the didban daemon",
				Value:   "http://127.0.0.1:9465",
				Sources: cli.EnvVars("DIDBAN_URL"),
			},
			&cli.IntFlag{
				Name:    "seconds",
				Aliases: []string{"s"},
				Usage:   "how long to hold the message",
				Value:   60,
			},
		},
		Action: run,
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	text := strings.Join(cmd.Args().Slice(), " ")
	if text == "" {
		return errors.New("lcd: no message given")
	}

	body, err := json.Marshal(map[string]any{"text": text, "seconds": cmd.Int("seconds")})
	if err != nil {
		return fmt.Errorf("lcd: encode request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := strings.TrimRight(cmd.String("url"), "/") + "/lcd"

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("lcd: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("lcd: post to %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("lcd: daemon returned %s", resp.Status)
	}

	return nil
}
