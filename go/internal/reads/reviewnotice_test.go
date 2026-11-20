package reads

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
)

// An agent whose last 10 looks at a kind of work were all OK at first look is offered one step down at that kind; Not now hides the offer
// for 10 more looks, a Redo among them keeps it away, and a level change starts it again. Another kind is offered on its own.
func TestTenFirstLookOKsInARowOfferAStepDownAndNotNowWaitsTenLooks(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	n := 0
	look := func(work string, ok bool) {
		t.Helper()
		n++
		run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(%d, 't', 'a1', 'p1', 1)", n))
		if err := db.AddReady(e.DB, int64(n), "a1", "a9", 0, db.KV{K: "works", V: []string{work}}, db.KV{K: "model", V: ""}); err != nil {
			t.Fatal(err)
		}
		v := db.VerdictOK
		if !ok {
			v = db.VerdictRedo
		}
		if err := db.AddVerdict(e.DB, int64(n), "p1", v, 0); err != nil {
			t.Fatal(err)
		}
	}
	offer := func() map[string]any { t.Helper(); return answerRow(t, e, "machine", "review:a1:screen") }
	look(db.WorkScreen, false)
	for i := 0; i < 9; i++ {
		look(db.WorkScreen, true)
	}
	look(db.WorkDocs, true) // another kind is no look at screen
	if offer() != nil {
		t.Fatal("an offer at nine in a row")
	}
	look(db.WorkScreen, true)
	r := offer()
	if r == nil {
		t.Fatal("no offer at ten in a row")
	}
	w, _ := r["warning"].(map[string]any)
	if r["title"] != "<@a1>'s screen results: look at 1 in 2?" || w["work"] != db.WorkScreen || w["level"] != db.LevelOneInTwo ||
		w["line"] != "Its last 10 screen results were OK at first look." {
		t.Fatalf("%v", r)
	}
	if answerRow(t, e, "machine", "review:a1:docs") != nil {
		t.Fatal("docs offered at one look")
	}
	if err := dismiss(t, e, "review:a1:screen"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 9; i++ {
		look(db.WorkScreen, true)
		if offer() != nil {
			t.Fatalf("back after %d more looks", i+1)
		}
	}
	look(db.WorkScreen, true)
	if offer() == nil {
		t.Fatal("not back after 10 more looks")
	}
	if err := db.SetReviewLevel(e.DB, "a1", db.WorkScreen, "", db.LevelNever, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	if offer() != nil {
		t.Fatal("an offer at never")
	}
}

// agents.list says each agent's least looked-at review level and its kind of work: every, with no kind, for one no row set.
func TestAgentsListSaysEachAgentsLeastLookedAtLevel(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "ann", "")
	addAgent(t, e, "bob", "")
	ann, bob := teamtest.Key(t, e.Home, "ann"), teamtest.Key(t, e.Home, "bob")
	model, _ := team.NewRoster(e.Home).Model(ann)
	for w, l := range map[string]string{db.WorkScreen: db.LevelOneInTwo, db.WorkDaemon: db.LevelOneInFive} {
		if err := db.SetReviewLevel(e.DB, ann, w, model, l, "p1", "set", 0); err != nil {
			t.Fatal(err)
		}
	}
	got := listed(t, e, nil)
	if got[ann]["review_level"] != db.LevelOneInFive || got[ann]["review_work"] != db.WorkDaemon ||
		got[bob]["review_level"] != db.LevelEvery || got[bob]["review_work"] != nil {
		t.Fatalf("ann %v %v, bob %v %v", got[ann]["review_level"], got[ann]["review_work"], got[bob]["review_level"], got[bob]["review_work"])
	}
}
