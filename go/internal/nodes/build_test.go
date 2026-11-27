package nodes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/runner"
)

// buildOf is a Build of two small files standing in for handlined and hl.
func buildOf(t *testing.T) *Build {
	t.Helper()
	dir := t.TempDir()
	for _, n := range runner.BuildNames {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("the "+n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b, err := OpenBuild(filepath.Join(dir, "handlined"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

// A runner of the master's platform on another version is offered the master's build with Auto on if it is the owner's; else only
// once its Update was asked; another platform never, and keeps "update to take work".
func TestTheMastersBuildIsOfferedOnAutoOrUpdate(t *testing.T) {
	r := registry(t)
	now := time.Now()
	r.SetClock(func() time.Time { return now })
	for _, q := range []string{"INSERT OR IGNORE INTO people(id, display_name, role, created_at) VALUES(1, 'Ron', 'owner', 0), (2, 'Kim', 'member', 0)",
		"UPDATE nodes SET person = 1 WHERE id = 'n1'"} {
		if _, err := r.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r.Build = buildOf(t)
	auto := true
	r.BuildAuto = func() bool { return auto }
	r.beats["n1"] = heard{at: now, beat: runner.Beat{Version: "v0", Platform: runner.Platform()}}
	o := r.offer("n1", r.beats["n1"].beat)
	if o == nil || o.Version != "v1" || len(o.Files) != 2 || o.Files[0].Name != "handlined" || o.Files[0].Size != int64(len("the handlined")) {
		t.Fatalf("auto: %+v", o)
	}
	if why := r.Takes("n1"); !strings.Contains(why, "starting again on the master's build") {
		t.Fatalf("auto, taking it: %q", why)
	}
	r.DB.Exec("UPDATE nodes SET person = 2 WHERE id = 'n1'")
	if o := r.offer("n1", r.beats["n1"].beat); o != nil {
		t.Fatalf("auto, another person's machine: %+v", o)
	}
	r.DB.Exec("UPDATE nodes SET person = 1 WHERE id = 'n1'")
	auto = false
	if o := r.offer("n1", r.beats["n1"].beat); o != nil {
		t.Fatalf("manual: %+v", o)
	}
	if why := r.Takes("n1"); !strings.Contains(why, "update to take work") {
		t.Fatalf("manual: %q", why)
	}
	if err := r.AskUpdate("n1"); err != nil {
		t.Fatal(err)
	}
	if o := r.offer("n1", r.beats["n1"].beat); o == nil {
		t.Fatal("manual, Update asked: no offer")
	}
	r.beats["n1"] = heard{at: now, beat: runner.Beat{Version: "v0", Platform: "plan9/mips"}}
	if o := r.offer("n1", r.beats["n1"].beat); o != nil {
		t.Fatalf("another platform: %+v", o)
	}
	if err := r.AskUpdate("n1"); err == nil || !strings.Contains(err.Error(), "install Handline there") {
		t.Fatalf("Update on another platform: %v", err)
	}
	r.beats["n1"] = heard{at: now, beat: runner.Beat{Version: "v0", Platform: runner.Platform(), BuildRefused: "its sha256 is x"}}
	if why := r.Takes("n1"); !strings.Contains(why, "refused the master's build: its sha256 is x") {
		t.Fatalf("refused: %q", why)
	}
}

// A Mac app with a newer version ready is told to install it on Auto when it is the owner's, else once its Update was asked; Update
// is not refused for a machine on the master's build whose app has one ready, and the ask ends once none is ready.
func TestTheAppsReadyVersionIsInstalledOnAutoOrUpdate(t *testing.T) {
	r := registry(t)
	now := time.Now()
	r.SetClock(func() time.Time { return now })
	for _, q := range []string{"INSERT OR IGNORE INTO people(id, display_name, role, created_at) VALUES(1, 'Ron', 'owner', 0), (2, 'Kim', 'member', 0)",
		"UPDATE nodes SET person = 2 WHERE id = 'n1'"} {
		if _, err := r.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r.Build = buildOf(t)
	auto := true
	r.BuildAuto = func() bool { return auto }
	beat := runner.Beat{Version: "v1", Platform: runner.Platform(), App: &runner.App{Version: "0.1.16", Ready: "0.1.17"}}
	r.beats["n1"] = heard{at: now, beat: beat}
	if u := r.appUpdate("n1", beat); u != nil {
		t.Fatalf("another person's Mac, not asked: %+v", u)
	}
	if err := r.AskUpdate("n1"); err != nil {
		t.Fatalf("Update with an app version ready, on the master's build: %v", err)
	}
	if u := r.appUpdate("n1", beat); u == nil || *u != (runner.AppUpdate{Version: "0.1.17"}) {
		t.Fatalf("asked: %+v", u)
	}
	if r.updates["n1"] {
		t.Fatal("the master's build was asked for a machine that runs it")
	}
	installed := runner.Beat{Version: "v1", Platform: runner.Platform(), App: &runner.App{Version: "0.1.17"}}
	if u := r.appUpdate("n1", installed); u != nil || r.appAsks["n1"] != "" {
		t.Fatalf("installed: %+v, asked %q", u, r.appAsks["n1"])
	}
	r.DB.Exec("UPDATE nodes SET person = 1 WHERE id = 'n1'")
	if u := r.appUpdate("n1", beat); u == nil || *u != (runner.AppUpdate{Version: "0.1.17", Auto: true}) {
		t.Fatalf("the owner's Mac on Auto: %+v", u)
	}
	auto = false
	if u := r.appUpdate("n1", beat); u != nil {
		t.Fatalf("Manual: %+v", u)
	}
	r.beats["n1"] = heard{at: now, beat: runner.Beat{Version: "v1", Platform: runner.Platform(), App: &runner.App{Version: "0.1.17"}}}
	if err := r.AskUpdate("n1"); err == nil || !strings.Contains(err.Error(), "runs the master's build already") {
		t.Fatalf("nothing to update: %v", err)
	}
}
