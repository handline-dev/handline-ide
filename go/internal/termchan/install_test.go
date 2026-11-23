package termchan

import (
	"errors"
	"testing"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/protocol/rpc"
)

func TestToolsInstallRunsTheMissingToolsFixInAShellWindow(t *testing.T) {
	r, h, _ := shellRig(t)
	r.e.Missing = func() map[string]string { return map[string]string{"git": "xcode-select --install"} }
	var re *rpc.Error
	for _, c := range []struct{ tool, code string }{{"make", "bad_request"}, {"claude", "not_found"}} {
		_, err := r.e.toolsInstall(person("casey"), map[string]any{"tool": c.tool, "fix": "rm -rf /"})
		if !errors.As(err, &re) || len(h.runs) != 0 {
			t.Fatalf("%s: %v %q", c.tool, err, h.runs)
		}
	}
	got, err := r.e.toolsInstall(person("casey"), map[string]any{"tool": "git", "command": "rm -rf /"})
	if err != nil {
		t.Fatal(err)
	}
	sh := got.(map[string]any)["terminal"].(string)
	if len(h.runs) != 1 || h.runs[0] != "install:git xcode-select --install" || h.panes[sh] != installStamp {
		t.Fatalf("%v %q %v", got, h.runs, h.panes)
	}
	h.ended = map[string]term.PaneRun{sh: {Tag: "install:git"}}
	if again, _ := r.e.toolsInstall(person("casey"), map[string]any{"tool": "git"}); again.(map[string]any)["terminal"] != sh || len(h.runs) != 1 {
		t.Fatalf("a running install is answered again: %v %q", again, h.runs)
	}
	h.ended[sh] = term.PaneRun{Tag: "install:git", Dead: true, Status: 1}
	again, _ := r.e.toolsInstall(person("casey"), map[string]any{"tool": "git"})
	if a := again.(map[string]any)["terminal"].(string); a == sh || len(h.runs) != 2 || len(h.closed) != 1 || h.closed[0] != sh {
		t.Fatalf("an ended install's window gives way to a new run: %v %q %v", again, h.runs, h.closed)
	}
	if _, err := r.e.shellClose(person("casey"), map[string]any{"terminal": again.(map[string]any)["terminal"].(string)}); err != nil {
		t.Fatal(err)
	}
}
