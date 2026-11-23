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
	if len(h.runs) != 1 || h.runs[0] != "install:git p='Install Command Line Developer Tools[.]app'; xcode-select --install"+installerWait || h.panes[sh] != installStamp {
		t.Fatalf("%v %q %v", got, h.runs, h.panes)
	}
	h.ended = map[string]term.PaneRun{sh: {Tag: "install:git"}, "%9": {Tag: "run:x"}}
	if got := r.e.Installs(); len(got) != 1 || got["git"] != (ToolInstall{Terminal: sh}) {
		t.Fatalf("a running install: %v", got)
	}
	if again, _ := r.e.toolsInstall(person("casey"), map[string]any{"tool": "git"}); again.(map[string]any)["terminal"] != sh || len(h.runs) != 1 {
		t.Fatalf("a running install is answered again: %v %q", again, h.runs)
	}
	h.ended[sh] = term.PaneRun{Tag: "install:git", Dead: true, Status: 1, DeadTime: 5}
	h.ended["%8"] = term.PaneRun{Tag: "install:git", Dead: true, DeadTime: 4}
	if got := r.e.Installs(); got["git"] != (ToolInstall{Terminal: sh, Ended: true}) {
		t.Fatalf("the install that ended last: %v", got)
	}
	delete(h.ended, "%8")
	h.ended[sh] = term.PaneRun{Tag: "install:git", Dead: true, Status: 1}
	again, _ := r.e.toolsInstall(person("casey"), map[string]any{"tool": "git"})
	if a := again.(map[string]any)["terminal"].(string); a == sh || len(h.runs) != 2 || len(h.closed) != 1 || h.closed[0] != sh {
		t.Fatalf("an ended install's window gives way to a new run: %v %q %v", again, h.runs, h.closed)
	}
	h.ended[again.(map[string]any)["terminal"].(string)] = term.PaneRun{Tag: "install:git"}
	if got := r.e.Installs(); got["git"].Ended || got["git"].Terminal == sh {
		t.Fatalf("a running install is told over an ended one: %v", got)
	}
	if _, err := r.e.shellClose(person("casey"), map[string]any{"terminal": again.(map[string]any)["terminal"].(string)}); err != nil {
		t.Fatal(err)
	}
}
