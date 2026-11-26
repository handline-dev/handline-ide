package session

import "testing"

func TestRunsScreencapture(t *testing.T) {
	for cmd, want := range map[string]bool{
		"screencapture -x /tmp/a.png":        true,
		"/usr/sbin/screencapture -l 5 a.png": true,
		"cd x && screencapture a.png":        true,
		"echo hi; screencapture":             true,
		"hl screen shot --app Finder":        false,
		"grep screencaptured notes":          false,
		"ls ~/screencapture-notes":           false,
	} {
		if got := RunsScreencapture("Bash", map[string]any{"command": cmd}); got != want {
			t.Errorf("%q: %v", cmd, got)
		}
	}
	if RunsScreencapture("Read", map[string]any{"command": "screencapture"}) {
		t.Error("only Bash")
	}
}
