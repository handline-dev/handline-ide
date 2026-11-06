package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/compose"
	"github.com/handline/handline/go/internal/daemon"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/sysres"
)

func TestTheDefaultHomeIsRefusedUnlessAskedFor(t *testing.T) {
	t.Setenv("HANDLINE_HOME", "")
	if h, err := resolveHome("", false); err == nil {
		t.Fatalf("started on the default home %s", h)
	}
	if h, err := resolveHome("/scratch/h", false); err != nil || h != "/scratch/h" {
		t.Fatalf("%q %v", h, err)
	}
	if _, err := resolveHome("", true); err != nil {
		t.Fatal(err)
	}
}

func TestTheRealHomeIsRefused(t *testing.T) {
	u, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	t.Setenv("HANDLINE_HOME", "")
	for _, d := range []string{".handline", ".handline/sub"} {
		if h, err := resolveHome(filepath.Join(u, d), false); err == nil {
			t.Fatalf("started on the real home %s", h)
		}
	}
	if _, err := resolveHome(filepath.Join(u, ".handline"), true); err != nil {
		t.Fatal(err)
	}
	a := t.TempDir()
	t.Setenv("HANDLINE_HOME", a)
	if _, err := resolveHome(a, false); err != nil {
		t.Fatal(err)
	}
}

func TestEachMissingAgentToolIsOneLine(t *testing.T) {
	var out bytes.Buffer
	log.SetOutput(&out)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	t.Setenv("PATH", t.TempDir())
	missingTools(filepath.Join(t.TempDir(), "claude"))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "claude not found") || !strings.Contains(lines[1], "git is not on PATH") || !strings.Contains(lines[2], "tmux is not on PATH") {
		t.Errorf("%q", out.String())
	}
}

// The app holds the write end of the daemon's stdin: closing it, as the app's exit does however it exits, stops the daemon.
func TestTheDaemonStopsWhenItsOwnersPipeCloses(t *testing.T) {
	r, w := io.Pipe()
	ctx := untilClosed(context.Background(), r)
	select {
	case <-ctx.Done():
		t.Fatal("stopped while the owner holds the pipe")
	case <-time.After(20 * time.Millisecond):
	}
	w.Close()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("still running after the pipe closed")
	}
}

// The daemon gets each part the composer serves it, the sleep lock the tick holds among them.
func TestTheDaemonGetsWhatThePartsServe(t *testing.T) {
	ccfg := compose.Config{Home: t.TempDir(), Log: func(string, ...any) {}}
	var cfg daemon.Config
	fromParts(&cfg, compose.New(ccfg), &ccfg)
	if cfg.Delivery == nil || cfg.KeepAwake == nil || cfg.Session == nil || cfg.BeforeClose == nil || cfg.Context == nil {
		t.Fatalf("%+v", cfg)
	}
	if held, _ := cfg.KeepAwake(); held {
		t.Fatal("held before any tick")
	}
}

// The agent views read the servers off the tracker the daemon serves the load from.
func TestTheViewsAndTheDaemonShareOneLoadTracker(t *testing.T) {
	ccfg := compose.Config{Home: t.TempDir()}
	var cfg daemon.Config
	shareLoad(&cfg, &ccfg)
	if cfg.Load == nil || ccfg.Servers == nil {
		t.Fatal("the tracker or the servers read is not set")
	}
	cfg.Load.Table = func() ([]procload.Raw, error) {
		return []procload.Raw{{PID: 7, PPID: 1, Start: 1, Cmd: "node a.js"}}, nil
	}
	cfg.Load.AgentOf = func(int) (string, bool) { return "alice", true }
	cfg.Load.Listen = func() ([]procload.Listener, error) {
		return []procload.Listener{{PID: 7, Addr: "127.0.0.1", Port: 80}}, nil
	}
	cfg.Load.Machine = func() (sysres.Machine, error) { return sysres.Machine{}, nil }
	cfg.Load.Disk = func(string) (int64, int64, error) { return 0, 0, nil }
	if err := cfg.Load.Sample(1000); err != nil {
		t.Fatal(err)
	}
	if got := ccfg.Servers("alice"); len(got) != 1 || got[0].Port != 80 {
		t.Fatalf("%+v", got)
	}
}
