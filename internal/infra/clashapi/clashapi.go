// Package clashapi reads a sing-box (or mihomo) Clash-compatible API: the
// open connections with their byte counters, and the proxy groups with their
// current pick and last probe delay. didban uses it to show the tunnel that
// runs on the same Pi.
package clashapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client talks to one API. Secret is the bearer token; empty sends none.
type Client struct {
	Base   string
	Secret string
	HTTP   *http.Client
}

// New returns a client for base (e.g. http://127.0.0.1:9099).
func New(base, secret string) *Client {
	return &Client{
		Base:   strings.TrimRight(base, "/"),
		Secret: secret,
		HTTP:   &http.Client{Timeout: 3 * time.Second},
	}
}

// Metadata is what the API knows about one connection's endpoints.
type Metadata struct {
	Network         string `json:"network"`
	Type            string `json:"type"`
	SourceIP        string `json:"sourceIP"`
	SourcePort      string `json:"sourcePort"`
	DestinationIP   string `json:"destinationIP"`
	DestinationPort string `json:"destinationPort"`
	Host            string `json:"host"`
}

// Connection is one open flow. Chains runs from the outbound to the group the
// rule picked, e.g. [htz02-hy2 auto tunnel].
type Connection struct {
	ID       string    `json:"id"`
	Metadata Metadata  `json:"metadata"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Start    time.Time `json:"start"`
	Chains   []string  `json:"chains"`
	Rule     string    `json:"rule"`
}

// Connections is the GET /connections body. The totals count every byte since
// the daemon started, closed connections included.
type Connections struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
}

// Delay is one probe result; 0 means the probe failed.
type Delay struct {
	Time  time.Time `json:"time"`
	Delay int       `json:"delay"`
}

// Proxy is one entry of GET /proxies: an outbound, or a group with Now and All.
type Proxy struct {
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	All     []string `json:"all"`
	History []Delay  `json:"history"`
}

// LastDelay is the newest probe result, nil when never probed.
func (p Proxy) LastDelay() *Delay {
	if len(p.History) == 0 {
		return nil
	}

	d := p.History[len(p.History)-1]

	return &d
}

// Connections fetches the open connections and the byte totals.
func (c *Client) Connections(ctx context.Context) (Connections, error) {
	var out Connections

	err := c.get(ctx, "/connections", &out)

	return out, err
}

// Proxies fetches every outbound and group, by tag.
func (c *Client) Proxies(ctx context.Context) (map[string]Proxy, error) {
	var out struct {
		Proxies map[string]Proxy `json:"proxies"`
	}

	err := c.get(ctx, "/proxies", &out)

	return out.Proxies, err
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	if c.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.Secret)
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, res.StatusCode)
	}

	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return fmt.Errorf("GET %s: decode: %w", path, err)
	}

	return nil
}
