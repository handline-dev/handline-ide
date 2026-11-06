package access

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/storetest"
)

func pathsOf(gs []Grant) []string {
	out := []string{}
	for _, g := range gs {
		out = append(out, g.Path+" "+g.Scope)
	}
	return out
}

// A second grant of a path replaces its scope; grants list newest first; revoke says whether there was one.
func TestPutReplacesAndRemoveSays(t *testing.T) {
	t.Parallel()
	d := storetest.Open(t)
	Put(d, "a", "/x", "once", 10)
	Put(d, "a", "/y", "always", 20)
	Put(d, "a", "/x", "always", 30)
	Put(d, "b", "/z", "once", 40)
	gs, _ := Grants(d, "a")
	if got := pathsOf(gs); !reflect.DeepEqual(got, []string{"/x always", "/y always"}) {
		t.Fatalf("%v", got)
	}
	if ok, _ := Remove(d, "a", "/x"); !ok {
		t.Fatal("removed false for a grant")
	}
	if ok, _ := Remove(d, "a", "/x"); ok {
		t.Fatal("removed true twice")
	}
	if gs, _ := Grants(d, "a"); !reflect.DeepEqual(pathsOf(gs), []string{"/y always"}) {
		t.Fatalf("%v", pathsOf(gs))
	}
}

// A once grant is the first fresh session's started after it: a resume of that session keeps it, a second fresh start ends it, and a used-up
// grant is neither listed nor kept by the next change. always stays.
func TestOnceGrantsLastOneSession(t *testing.T) {
	t.Parallel()
	d := storetest.Open(t)
	start := func(resumed int) {
		if _, err := d.Exec("INSERT INTO session_starts(agent, ts, cause, resumed, text, prompt) VALUES('a', 1, 'wake', ?, '', '')", resumed); err != nil {
			t.Fatal(err)
		}
	}
	start(0) // a session from before the grant
	Put(d, "a", "/once", "once", 10)
	Put(d, "a", "/always", "always", 5)
	both, one := []string{"/once", "/always"}, []string{"/always"}
	check := func(step string, resuming bool, want []string) {
		t.Helper()
		if got, err := ForLaunch(d, "a", resuming); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %v %v", step, got, err)
		}
	}
	check("before any start", false, both)
	start(0)
	check("a fresh start after the one that took it", false, one)
	check("a resume of the one that took it", true, both)
	start(1)
	check("after a resume", true, both)
	if gs, _ := Grants(d, "a"); len(gs) != 2 {
		t.Fatalf("listed while its session may run: %v", gs)
	}
	start(0)
	check("a second fresh start", true, one)
	if gs, _ := Grants(d, "a"); !reflect.DeepEqual(pathsOf(gs), []string{"/always always"}) {
		t.Fatalf("%v", pathsOf(gs))
	}
	if ok, _ := Remove(d, "a", "/once"); ok {
		t.Fatal("a used-up grant removed")
	}
	if gs, _ := stored(d, "a"); len(gs) != 1 {
		t.Fatalf("a change kept the used-up grant: %v", gs)
	}
}

// A once grant the running session was given is that session's: the next fresh start does not take it, and it ends there.
func TestAOnceGrantGivenNowIsTheRunningSessions(t *testing.T) {
	t.Parallel()
	d := storetest.Open(t)
	for _, resumed := range []int{0, 1} { // the running session started, then was resumed
		d.Exec("INSERT INTO session_starts(agent, ts, cause, resumed, text, prompt) VALUES('a', 1, 'wake', ?, '', '')", resumed)
	}
	Put(d, "a", "/next", "once", 9)
	Put(d, "a", "/now", "once", 10)
	if err := GivenNow(d, "a", "/now"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ForLaunch(d, "a", true); len(got) != 2 {
		t.Fatalf("a resume of the running session: %v", got)
	}
	if got, _ := ForLaunch(d, "a", false); fmt.Sprint(got) != "[/next]" {
		t.Fatalf("the next fresh session: %v", got)
	}
	d.Exec("INSERT INTO session_starts(agent, ts, cause, resumed, text, prompt) VALUES('a', 1, 'wake', 0, '', '')")
	if gs, _ := Grants(d, "a"); len(gs) != 1 || gs[0].Path != "/next" {
		t.Fatalf("after the next fresh start: %v", gs)
	}
}

// Roots are the worktree, the agent folder, the scratch root, the attachments folder and the grants; a folder covers what is under it,
// a path through a link is where the link leads (/tmp is /private/tmp), and /usr does not hold /usrx.
func TestOutside(t *testing.T) {
	t.Parallel()
	d := storetest.Open(t)
	home, wt := t.TempDir(), t.TempDir()
	if err := db.SetTerminal(d, "a", "t1", wt); err != nil {
		t.Fatal(err)
	}
	Put(d, "a", "/usr", "once", 1)
	roots, err := Roots(d, home, "a")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wt, "f"), []byte("x"), 0o600)
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(wt, link)
	os.Symlink("/etc", filepath.Join(wt, "etc"))
	in := []string{wt + "/src/x.go", paths.AgentDir(home, "a") + "/memory", filepath.Join(home, "attachments", "f.png"), "/usr/bin",
		"/usr", paths.ScratchRoot() + "/s", link + "/f"}
	out := []string{"/usrx", "/etc/hosts", home + "/db.sqlite", wt + "x", wt + "/etc/hosts"}
	if got := Outside(roots, append(append([]string{}, in...), out...)); !reflect.DeepEqual(got, out) {
		t.Fatalf("%v, want %v", got, out)
	}
	if got := Outside([]string{"/"}, out); got != nil {
		t.Fatalf("a grant of / leaves %v outside", got)
	}
}
