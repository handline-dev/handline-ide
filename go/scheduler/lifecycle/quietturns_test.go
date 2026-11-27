package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

// The launch asks the vendor for no reminder after quiet turns only when the setting says so: every agent's, and an agent's own over it.
func TestTheLaunchAsksForNoSilenceReminderWhenTheSettingSaysSo(t *testing.T) {
	r := newLaunchRig(t)
	quiet := func() bool {
		t.Helper()
		_, env, err := r.build("ann", nil, false, "main")
		if err != nil {
			t.Fatal(err)
		}
		return env["QUIET_TURNS"] == "1"
	}
	global := func(on string) {
		t.Helper()
		path := paths.ConfigFile(r.l.Home)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte("no_silence_reminder: "+on+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if quiet() {
		t.Fatal("off by default")
	}
	global("true")
	if !quiet() {
		t.Fatal("every agent's switch is on")
	}
	teamtest.Set(t, r.l.Home, "ann", "no_silence_reminder", false)
	if quiet() {
		t.Fatal("the agent's own off is over every agent's on")
	}
	global("false")
	teamtest.Set(t, r.l.Home, "ann", "no_silence_reminder", true)
	if !quiet() {
		t.Fatal("the agent's own on is over every agent's off")
	}
}
