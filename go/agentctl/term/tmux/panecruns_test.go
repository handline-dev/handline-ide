//go:build !windows

package tmux

import (
	"github.com/handline/handline/go/agentctl/term"

	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A tmux server that is not running has no windows, which is an answer: the runs in it are over.
func TestPaneRunsOfNoServerIsEmptyNotUnknown(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	got := Tmux{term.Place{Socket: "agentctl-test-" + filepath.Base(t.TempDir())}}.PaneRuns()
	if got == nil || len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestSignalNumberReadsWhatEveryTmuxCallsASignal(t *testing.T) {
	check := func(want int, names ...string) {
		for _, name := range names {
			if got := signalNumber(name); got != want {
				t.Errorf("%q: %d, want %d", name, got, want)
			}
		}
	}
	check(2, "int", "INT", "SIGINT", "2", "interrupt")
	check(15, "sigterm", "15", "Terminated")
	check(0, "", "nonsense")
}

// A listing line PaneRuns cannot read is "cannot say", never "no window": skipped, the command running there would be told stopped. The
// lines a tmux with no UTF-8 locale prints, every tab drawn as "_", are such lines.
func TestAListingLineThatCannotBeReadMakesTheAnswerUnknown(t *testing.T) {
	if got := parsePaneRuns("%16\tp5:0@1\t0\t\t\t\n%3\t\t0\t\t\t\n"); len(got) != 1 || got["%16"].Tag != "p5:0@1" {
		t.Fatalf("a readable listing: %v", got)
	}
	if got := parsePaneRuns(""); got == nil || len(got) != 0 {
		t.Fatalf("a server with no windows: %v", got)
	}
	if got := parsePaneRuns("%16_p5:0@1_0___\n"); got != nil {
		t.Fatalf("a listing with its tabs drawn as _: %v", got)
	}
}

// A daemon an app starts has no locale: tmux still answers it with tabs, so its run windows are listed.
func TestARunWindowIsListedForAProgramWithNoLocale(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	for _, v := range []string{"LANG", "LC_ALL", "LC_CTYPE", "TMUX"} { // a client inside tmux takes UTF-8 for granted
		t.Setenv(v, "")
		os.Unsetenv(v)
	}
	tm := Tmux{term.Place{Socket: "agentctl-test-" + filepath.Base(t.TempDir())}}
	t.Cleanup(func() { tm.tmux(tmuxTimeout, "", "kill-server") })
	run, why := tm.CreateRun(t.TempDir(), "ann-run", "ann", "9:0@1", "sleep 30", true)
	if run == "" {
		t.Fatal(why)
	}
	if got := tm.PaneRuns(); got[run].Tag != "9:0@1" {
		t.Fatalf("the run window is not listed: %v", got)
	}
}
