package tunnel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1995parham-me/didban/internal/infra/clashapi"
	"github.com/1995parham-me/didban/internal/tunnel"
)

type fake struct {
	conns   clashapi.Connections
	proxies map[string]clashapi.Proxy
	err     error
}

func (f *fake) Connections(context.Context) (clashapi.Connections, error) { return f.conns, f.err }

func (f *fake) Proxies(context.Context) (map[string]clashapi.Proxy, error) { return f.proxies, f.err }

func proxies() map[string]clashapi.Proxy {
	at := time.Now()

	return map[string]clashapi.Proxy{
		"tunnel":    {Type: "Selector", Now: "auto", All: []string{"auto", "htz02-hy2", "htz02", "direct"}},
		"auto":      {Type: "URLTest", Now: "htz02-hy2", All: []string{"htz02-hy2", "htz02"}},
		"htz02-hy2": {Type: "Hysteria2", History: []clashapi.Delay{{Time: at, Delay: 300}}},
		"htz02":     {Type: "VLESS", History: []clashapi.Delay{{Time: at, Delay: 0}}},
		"direct":    {Type: "Direct"},
	}
}

func TestPoll(t *testing.T) {
	t.Parallel()

	start := time.Now().Add(-time.Minute)
	src := &fake{proxies: proxies(), conns: clashapi.Connections{
		DownloadTotal: 1000,
		UploadTotal:   100,
		Connections: []clashapi.Connection{
			{ID: "a", Download: 500, Start: start, Chains: []string{"htz02-hy2", "auto", "tunnel"}, Metadata: clashapi.Metadata{Host: "github.com"}},
		},
	}}

	w := tunnel.New(src, "tunnel", 10)
	if err := w.Poll(t.Context()); err != nil {
		t.Fatal(err)
	}

	s := w.Snapshot()
	if s.Exit != "htz02-hy2" || s.Now != "auto" {
		t.Fatalf("exit %q now %q", s.Exit, s.Now)
	}

	if len(s.Exits) != 3 || !s.Exits[0].Active || s.Exits[0].Name != "htz02-hy2" || s.Exits[2].Name != "direct" {
		t.Fatalf("exits %+v", s.Exits)
	}

	if s.DownBps != 0 || s.Flows[0].DownBps != 0 {
		t.Fatalf("first poll has no rate: %+v", s)
	}

	src.conns.DownloadTotal = 3000
	src.conns.Connections[0].Download = 1500
	src.conns.Connections = append(src.conns.Connections, clashapi.Connection{ID: "b", Download: 10, Start: start})

	if err := w.Poll(t.Context()); err != nil {
		t.Fatal(err)
	}

	s = w.Snapshot()
	if s.DownBps <= 0 || len(s.Flows) != 2 || s.Flows[0].ID != "a" || s.Flows[0].DownBps <= s.Flows[1].DownBps {
		t.Fatalf("second poll: %+v", s)
	}

	src.err = errors.New("down")
	if err := w.Poll(t.Context()); err == nil {
		t.Fatal("want error")
	}

	if s = w.Snapshot(); s.OK || s.Flows != nil || s.Error == "" {
		t.Fatalf("failed poll: %+v", s)
	}
}
