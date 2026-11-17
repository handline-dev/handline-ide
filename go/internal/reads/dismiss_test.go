package reads

import (
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/prefs"
)

func dismiss(t *testing.T, e *Env, id string) error {
	t.Helper()
	now, err := e.WarningSinces()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := e.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := prefs.Dismiss(tx, id, now); err != nil {
		return err
	}
	return tx.Commit()
}

func needLogin(t *testing.T, e *Env, since int64) {
	t.Helper()
	if err := db.SetLoginNeed(e.DB, "alpha", db.LoginNeed{Since: since}); err != nil {
		t.Fatal(err)
	}
}

// A dismissed warning stays gone while its cause lasts, and the same warning raised again is a new row.
func TestADismissedWarningStaysGoneUntilItsCauseBeginsAgain(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	if err := dismiss(t, e, "login:alpha"); err == nil {
		t.Fatal("a warning the founder does not have was dismissed")
	}
	needLogin(t, e, 100)
	if err := dismiss(t, e, "login:alpha"); err != nil {
		t.Fatal(err)
	}
	gone := func(why string) {
		t.Helper()
		if r := answerRow(t, e, "machine", "login:alpha"); r != nil {
			t.Fatalf("%s: %v", why, r)
		}
		if items, count, err := e.ForYou(); err != nil || count != 0 {
			t.Fatalf("%s: for_you %v %d %v", why, items, count, err)
		}
	}
	gone("dismissed")
	needLogin(t, e, 100) // the watch keeps the need, as on every tick and every start
	gone("the cause still holds")
	if err := db.ClearMeta(e.DB, db.LoginNeedPrefix+"alpha"); err != nil {
		t.Fatal(err)
	}
	needLogin(t, e, 200)
	if r := answerRow(t, e, "machine", "login:alpha"); r == nil {
		t.Fatal("the login needed again did not come back")
	}
}

// A login warning dismissed comes back when it grows worse: the account cannot be used, which the watch writes keeping its since.
func TestADismissedLoginWarningComesBackWhenTheAccountCannotBeUsed(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	needLogin(t, e, 100)
	if err := dismiss(t, e, "login:alpha"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetLoginNeed(e.DB, "alpha", db.LoginNeed{Since: 100, Unusable: "billing_error"}); err != nil {
		t.Fatal(err)
	}
	if r := answerRow(t, e, "machine", "login:alpha"); r == nil {
		t.Fatal("the worse warning stayed hidden")
	}
}
