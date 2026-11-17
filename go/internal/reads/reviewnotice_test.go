package reads

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
)

// An agent 8 of whose last 10 looks were OK at first look raises the founder's notice that it could be checked less; Not now hides it for
// 5 more looks, and a level change starts it again.
func TestAnAgentThatLearnedEnoughRaisesTheNoticeAndNotNowWaitsFiveLooks(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	n := 0
	look := func(ok bool) {
		t.Helper()
		n++
		run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(%d, 't', 'a1', 'p1', 1)", n))
		if err := db.AddReady(e.DB, int64(n), "a1", "a9", 0); err != nil {
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
	notice := func() map[string]any { t.Helper(); return answerRow(t, e, "machine", "review:a1") }
	for i := 0; i < 9; i++ {
		look(i != 2)
	}
	if notice() != nil {
		t.Fatal("a notice at nine looks")
	}
	look(false) // 8 of 10
	r := notice()
	if r == nil {
		t.Fatal("no notice at 8 of 10")
	}
	w, _ := r["warning"].(map[string]any)
	if !strings.Contains(fmt.Sprint(r["title"]), "has learned enough to be checked less") || w["line"] != "0 lessons. 8 of its last 10 tasks were OK at first look." ||
		w["level"] != db.LevelOneInThree {
		t.Fatalf("%v", r)
	}
	if err := dismiss(t, e, "review:a1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		look(true)
		if notice() != nil {
			t.Fatalf("back after %d more looks", i+1)
		}
	}
	look(true)
	if notice() == nil {
		t.Fatal("not back after 5 more looks")
	}
	if err := db.SetReviewLevel(e.DB, "a1", db.LevelAuto, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	if notice() != nil {
		t.Fatal("a notice at auto")
	}
}

// agents.list says each agent's review level: every for one no row set.
func TestAgentsListSaysEachAgentsReviewLevel(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "ann", "")
	addAgent(t, e, "bob", "")
	ann, bob := teamtest.Key(t, e.Home, "ann"), teamtest.Key(t, e.Home, "bob")
	if err := db.SetReviewLevel(e.DB, ann, db.LevelAuto, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	got := listed(t, e, nil)
	if got[ann]["review_level"] != db.LevelAuto || got[bob]["review_level"] != db.LevelEvery {
		t.Fatalf("ann %v, bob %v", got[ann]["review_level"], got[bob]["review_level"])
	}
}
