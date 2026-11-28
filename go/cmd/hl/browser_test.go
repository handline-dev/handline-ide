package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/handline/handline/go/internal/browser"
)

// aBrowserRuns records a stand-in for the agent's Chrome: a process whose command line names its profile, and a DevTools port that
// answers /json/version and nothing else, so a verb that reached the page would fail.
func aBrowserRuns(t *testing.T, home, agent string) {
	t.Helper()
	dir := browser.AgentDir(home, agent)
	prof := filepath.Join(dir, "profile")
	if err := os.MkdirAll(prof, 0o700); err != nil {
		t.Fatal(err)
	}
	proc := exec.Command("sh", "-c", "sleep 60; :", "--user-data-dir="+prof)
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proc.Process.Kill(); proc.Wait() })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.Error(w, "no page here", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	n, _ := strconv.Atoi(port)
	state, _ := json.Marshal(map[string]any{"pid": proc.Process.Pid, "port": n})
	if err := os.WriteFile(filepath.Join(dir, "state.json"), state, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEveryBrowserVerbAsksFirstAndStopsWhileAPersonHasThePage(t *testing.T) {
	r := newRig(t)
	aBrowserRuns(t, r.home, "a.one")
	calls := r.daemon(map[string]string{"screen.act": "error[input_holder]:p1 has the page"})
	for _, args := range [][]string{{"open", "https://example.com"}, {"read"}, {"read", "--tree"}, {"click", "#b"}, {"type", "#n", "secret"},
		{"wait", "#b"}, {"eval", "1"}, {"shot", "x.png"}, {"close"}} {
		_, errs, code := r.run(asAgent, append([]string{"browser"}, args...)...)
		if code != 1 || errs != "hl browser "+args[0]+": p1 has the page\n" {
			t.Errorf("%v while held: exit %d, %q", args, code, errs)
		}
		last := sent(calls())
		want := fmt.Sprintf(`screen.act {"kind":%q,"source":"browser"}`, args[0])
		if got := last[len(last)-1]; got != want {
			t.Errorf("%v asked %s, want %s", args, got, want)
		}
	}
}
