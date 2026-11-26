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

// A dismissed stall notice stays gone when the daemon's start retry clears the stall and a failed retry raises it again; it shows again
// only once the agent has started a session since.
func TestADismissedStallStaysGoneOverARetryUntilTheAgentRuns(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	stall := func(at int64) {
		t.Helper()
		run(t, e, "INSERT OR REPLACE INTO agent_state(agent, state, updated) VALUES('bob', 'STALLED', 1)")
		if err := db.NoteFounderNotice(e.DB, db.FounderNotice{Agent: "bob", Reason: "start_stall", Text: "bob is STALLED", At: at}); err != nil {
			t.Fatal(err)
		}
	}
	const id = "notice:bob:start_stall"
	stall(200)
	if err := dismiss(t, e, id); err != nil {
		t.Fatal(err)
	}
	if r := answerRow(t, e, "machine", id); r != nil {
		t.Fatalf("dismissed: %v", r)
	}
	run(t, e, "UPDATE agent_state SET state = 'IDLE' WHERE agent = 'bob'") // the retry at the daemon's start
	stall(300)
	if r := answerRow(t, e, "machine", id); r != nil {
		t.Fatalf("a failed retry brought it back: %v", r)
	}
	run(t, e, "INSERT INTO events(ts, agent, kind) VALUES(400, 'bob', 'SessionStart')")
	stall(500)
	r := answerRow(t, e, "machine", id)
	if r == nil {
		t.Fatal("a stall after the agent ran again is a new one")
	}
	if w, _ := r["warning"].(map[string]any); w["stall"] != true || w["agent"] != "bob" {
		t.Errorf("the row does not offer Try again on bob: %v", r["warning"])
	}
}
