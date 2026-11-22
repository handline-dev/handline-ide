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
	r.Clock = func() time.Time { return now }
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
