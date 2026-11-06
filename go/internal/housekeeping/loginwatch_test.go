package housekeeping

import (
	"fmt"
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
	in, known, asked := false, true, 0
	w := &LoginWatch{Home: home, State: func(string) (bool, bool) { asked++; return in, known }}
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
	if _, err := q.Exec("UPDATE meta SET updated = 0 WHERE name = ?", db.LoginNeedPrefix+"alpha"); err != nil {
		t.Fatal(err)
	}
	check()
	if need() != first {
		t.Fatalf("since moved from %s to %s", first, need())
	}
	var updated float64
	if err := q.QueryRow("SELECT updated FROM meta WHERE name = ?", db.LoginNeedPrefix+"alpha").Scan(&updated); err != nil || updated != 0 {
		t.Fatalf("an unchanged need was written again (the watchers follow meta): %v %v", updated, err)
	}
	in = true
	check()
	if need() != "" {
		t.Fatal("a login ends it")
	}
	in, known = false, false
	check()
	if need() != "" {
		t.Fatal("a login this process cannot tell asks nothing")
	}
	known = true
	endOn := func(day string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("sub_end_alpha: \""+day+"\"\nmonth_cycle_alpha: true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	endOn(time.Unix(now, 0).Format("2006-01-02"))
	check()
	if need() == "" {
		t.Fatal("an account on its last day still wants a login")
	}
	endOn("2020-01-01")
	check()
	if need() != "" {
		t.Fatal("an account past its last day asks nothing")
	}
}

// A stall keeps its time and why the account cannot be used, so the read that shows it asks nothing of the account.
func TestTheLoginWatchKeepsWhyAStalledAccountCannotBeUsed(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "accounts", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	q, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	at := time.Now().Unix() + 1000 // after any credential, so the stall stands
	stall := func(errType string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "login_expired.json"), []byte(fmt.Sprintf(`{"why":"x","error":%q,"at":%d}`, errType, at)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	need := func() db.LoginNeed {
		t.Helper()
		v, _, err := db.Meta(q, db.LoginNeedPrefix+"alpha")
		n, ok := db.ReadLoginNeed(v)
		if err != nil || !ok {
			t.Fatalf("no need kept: %q %v", v, err)
		}
		return n
	}
	w := &LoginWatch{Home: home, State: func(string) (bool, bool) { return false, true }}
	now := time.Now().Unix()
	stall("authentication_failed")
	if err := w.Check(q, now); err != nil {
		t.Fatal(err)
	}
	if n := need(); n != (db.LoginNeed{Since: at}) {
		t.Fatalf("an expired login: %+v", n)
	}
	stall("billing_error")
	if err := w.Check(q, now+LoginCheckEveryS); err != nil {
		t.Fatal(err)
	}
	if n := need(); n != (db.LoginNeed{Since: at, Unusable: "billing_error"}) {
		t.Fatalf("a stall on another error: %+v", n)
	}
}
