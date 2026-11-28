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
// answers /json/version, /json/list with pages when given, and nothing else, so a verb that reached the page would fail.
func aBrowserRuns(t *testing.T, home, agent string, pages ...string) {
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
		if r.URL.Path == "/json/list" && len(pages) > 0 {
			fmt.Fprint(w, pages[0])
			return
		}
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
		{"wait", "#b"}, {"eval", "1"}, {"shot", "x.png"}, {"press", "Enter"}, {"hover", "#b"}, {"drag", "#a", "#b"}, {"select", "#s", "Red"},
		{"scroll", "down"}, {"back"}, {"forward"}, {"reload"}, {"tab"}, {"tab", "new"}, {"tab", "2"}, {"tab", "close", "2"},
		{"viewport", "390", "844"}, {"upload", "#f", "a.pdf"}, {"download", "#d"}, {"dialog", "accept"}, {"dialog", "dismiss"}, {"console"},
		{"network"}, {"close"}} {
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

func TestBrowserVerbsRefuseWordsTheyDoNotTake(t *testing.T) {
	r := newRig(t)
	aBrowserRuns(t, r.home, "a.one")
	r.daemon(nil)
	for _, args := range [][]string{{"scroll", "sideways"}, {"scroll", "down", "-5"}, {"tab", "close"}, {"tab", "zero"}, {"tab", "new", "a", "b"},
		{"dialog", "maybe"}, {"dialog", "dismiss", "x"}, {"viewport", "0", "5"}, {"upload", "#f"}, {"drag", "#a"}, {"press"}, {"back", "1"}} {
		if _, errs, code := r.run(asAgent, append([]string{"browser"}, args...)...); code != 2 || !strings.Contains(errs, "arguments not understood") {
			t.Errorf("%v: exit %d, %q", args, code, errs)
		}
	}
}

func TestUploadAndDownloadStayInTheWorktree(t *testing.T) {
	top := t.TempDir()
	t.Chdir(top)
	if _, err := inWorktree([]string{"."}); err == nil {
		t.Error("a folder outside any checkout was taken")
	}
	if err := exec.Command("git", "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile("in.txt", nil, 0o600)
	os.WriteFile(filepath.Join(filepath.Dir(top), "out.txt"), nil, 0o600)
	if got, err := inWorktree([]string{"in.txt", "."}); err != nil || len(got) != 2 || filepath.Base(got[0]) != "in.txt" {
		t.Errorf("inside = %v, %v", got, err)
	}
	for _, p := range []string{"../out.txt", "/etc/hosts", "gone.txt"} {
		if _, err := inWorktree([]string{p}); err == nil {
			t.Errorf("%s was taken", p)
		}
	}
}
