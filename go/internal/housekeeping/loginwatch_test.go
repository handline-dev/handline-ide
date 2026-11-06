package housekeeping

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/storetest"
)

func TestTheLoginWatchHoldsANeedUntilTheLoginOrTheLastDay(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "accounts", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	q, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	in, asked := false, 0
	w := &LoginWatch{Home: home, State: func(string) (bool, bool) { asked++; return in, true }}
	now := time.Now().Unix()
	need := func() string {
		v, _, err := db.Meta(q, db.LoginNeedPrefix+"alpha")
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	check := func() {
		t.Helper()
		if err := w.Check(q, now); err != nil {
			t.Fatal(err)
		}
		now += LoginCheckEveryS
	}
	check()
	first := need()
	if first == "" {
		t.Fatal("no need kept")
	}
	if err := w.Check(q, now-1); err != nil || asked != 1 {
		t.Fatalf("the keychain is asked once a minute: %d %v", asked, err)
	}
	check()
	if need() != first {
		t.Fatalf("since moved from %s to %s", first, need())
	}
	in = true
	check()
	if need() != "" {
		t.Fatal("a login ends it")
	}
	in = false
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("sub_end_alpha: \"2020-01-01\"\nmonth_cycle_alpha: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	check()
	if need() != "" {
		t.Fatal("an account past its last day asks nothing")
	}
}
