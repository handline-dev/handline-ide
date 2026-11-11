package taskwrites

import (
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/storetest"
)

func TestAnyoneMayEditOrReassignATask(t *testing.T) {
	t.Parallel()
	db, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	home := t.TempDir()
	teamtest.Seed(t, home, "shop", "shop.a", "shop.b", "other", "other.c")
	homes.Store(db, home)
	k := func(p *string) *string { // an edit's who, by key as a handler gets it
		if p == nil {
			return nil
		}
		v := teamtest.Key(t, home, *p)
		return &v
	}
	if _, err := db.Exec(keyed(db, "INSERT INTO tasks(id, title, owner, opened_by, created_at, reporter) VALUES(1, 'for other', 'shop.b', 'other.c', 1, 'other')")); err != nil {
		t.Fatal(err)
	}
	s := func(v string) *string { return &v }
	edit := func(by string, owner, reporter, title *string) string {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Commit()
		why, _, err := editTask(tx, 1, teamtest.Key(t, home, by), home, k(owner), k(reporter), title, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return why
	}
	read := func() (owner, reporter, answers string) {
		tx, _ := db.Begin()
		defer tx.Rollback()
		tk, _ := getTask(tx, 1)
		answers, _ = asker(tx, tk)
		return named(db, tk.Owner), named(db, tk.reporter()), named(db, answers)
	}
	if _, _, a := read(); a != "other" {
		t.Fatalf("a set reporter is who the task answers to: %q", a)
	}
	if why := edit("other", nil, nil, s("renamed")); why != "" {
		t.Fatalf("the reporter may edit: %s", why)
	}
	if why := edit("shop.a", s("shop.a"), nil, nil); why != "" {
		t.Fatalf("anyone may take it on: %s", why)
	}
	if why := edit("shop", nil, s("shop"), nil); why != "" {
		t.Fatalf("anyone may change who answers for it: %s", why)
	}
	if why := edit("shop.a", s("shop.b"), nil, s("both")); why != "" {
		t.Fatalf("anyone may reshape it too: %s", why)
	}
	if owner, reporter, _ := read(); owner != "shop.b" || reporter != "shop" {
		t.Fatalf("owner %q reporter %q", owner, reporter)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM events WHERE kind = 'edit_task' AND agent = 'shop' AND json_extract(payload, '$.reporter.new') = 'shop'"); n != 1 {
		t.Fatalf("each change is kept with who made it: %d", n)
	}
}
