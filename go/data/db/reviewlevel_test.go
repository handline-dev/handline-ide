package db

import (
	"fmt"
	"slices"
	"testing"

	"github.com/handline/handline/go/kit/gitrun"
)

// readyAs makes a task agent finished as Ready, of works while it ran model, judged at the first of them.
func readyAs(t *testing.T, q Q, agent, model string, works ...string) int64 {
	t.Helper()
	tid := seedTask(t, q, agent, "p1")
	if err := AddReady(q, tid, agent, "a9", 0, KV{"may_skip", true}, KV{"works", works}, KV{"model", model}, KV{"judged_by", works[0]}); err != nil {
		t.Fatal(err)
	}
	return tid
}

// The record is the first verdict by a person on each task the agent made Ready of that kind of work under its model now, after its level
// there was set: an automatic OK is no look, a later verdict on the same task is no second look, another kind or model is not counted, and a
// level change starts it again.
func TestTheReviewRecordCountsFirstLooksAtTheLevelNow(t *testing.T) {
	q := open(t)
	verdict := func(tid int64, by, v string) {
		t.Helper()
		if err := AddVerdict(q, tid, by, v, 0); err != nil {
			t.Fatal(err)
		}
	}
	task := func() int64 { return readyAs(t, q, "a1", "m1", WorkScreen, WorkDaemon) }
	old := task()
	verdict(old, "p1", VerdictRedo)
	if err := SetReviewLevel(q, "a1", WorkScreen, "m1", LevelOneInThree, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	verdict(old, "p1", VerdictOK) // its first look was before the level
	one, two, three := task(), task(), task()
	docs, otherModel, other := readyAs(t, q, "a1", "m1", WorkDocs), readyAs(t, q, "a1", "m2", WorkScreen), readyAs(t, q, "a2", "m1", WorkScreen)
	verdict(one, "p1", VerdictOK)
	verdict(two, "p1", VerdictRedo)
	verdict(two, "p1", VerdictOK)
	verdict(three, AutoVerdictBy, VerdictOK)
	for _, tid := range []int64{docs, otherModel, other} {
		verdict(tid, "p1", VerdictOK)
	}
	l, _ := ReviewLevel(q, "a1", WorkScreen, "m1")
	if l.Level != LevelOneInThree {
		t.Fatalf("level %+v", l)
	}
	rec, err := ReviewRecord(q, "a1", "m1", l)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(rec); got != fmt.Sprint([]Look{{one, true, 1000}, {two, false, 1000}}) {
		t.Fatalf("record %s", got)
	}
	if err := SetReviewLevel(q, "a1", WorkScreen, "m1", LevelEvery, "a1", "redo", two); err != nil {
		t.Fatal(err)
	}
	l, _ = ReviewLevel(q, "a1", WorkScreen, "m1")
	if rec, _ := ReviewRecord(q, "a1", "m1", l); len(rec) != 0 {
		t.Fatalf("a new level kept %v", rec)
	}
	if l, _ := ReviewLevel(q, "a2", WorkScreen, "m1"); l.Level != LevelEvery || l.Row != 0 {
		t.Fatalf("an agent no row set: %+v", l)
	}
	if w, _ := AgentWorks(q, "a1"); !slices.Equal(w, []string{WorkScreen, WorkDaemon, WorkDocs}) {
		t.Fatalf("a1 has done %v", w)
	}
}

// A level is per kind of work and holds only under the model it was set with; contract and data is every whatever a row says.
func TestALevelIsPerKindAndModelAndContractIsAlwaysEvery(t *testing.T) {
	q := open(t)
	for _, w := range []string{WorkScreen, WorkContract} {
		if err := SetReviewLevel(q, "a1", w, "m1", LevelNever, "p1", "set", 0); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		work, model, want string
	}{
		{WorkScreen, "m1", LevelNever},
		{WorkDaemon, "m1", LevelEvery},   // another kind
		{WorkScreen, "m2", LevelEvery},   // the model changed
		{WorkContract, "m1", LevelEvery}, // the floor
	} {
		if l, _ := ReviewLevel(q, "a1", c.work, c.model); l.Level != c.want || l.Work != c.work {
			t.Errorf("%s under %s: %+v, want %s", c.work, c.model, l, c.want)
		}
	}
}

// The sampling count is of may-skip finishes judged at that kind under that model since the level was set.
func TestMaySkipCountIsPerKindAndModel(t *testing.T) {
	q := open(t)
	readyAs(t, q, "a1", "m1", WorkScreen)
	if err := SetReviewLevel(q, "a1", WorkScreen, "m1", LevelOneInTwo, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	l, _ := ReviewLevel(q, "a1", WorkScreen, "m1")
	readyAs(t, q, "a1", "m1", WorkScreen, WorkDocs)
	readyAs(t, q, "a1", "m1", WorkDaemon)
	readyAs(t, q, "a1", "m2", WorkScreen)
	readyAs(t, q, "a2", "m1", WorkScreen)
	if n, err := MaySkipCount(q, "a1", WorkScreen, "m1", l.After); err != nil || n != 1 {
		t.Fatalf("count %d %v, want 1", n, err)
	}
}

// Learned enough is the last 10 all OK at first, never at never nor for contract and data.
func TestLearnedEnoughIsTenInARow(t *testing.T) {
	looks := func(oks ...bool) []Look {
		var out []Look
		for _, ok := range oks {
			out = append(out, Look{OKFirst: ok})
		}
		return out
	}
	y, n := true, false
	for _, c := range []struct {
		l    LevelAt
		rec  []Look
		want bool
	}{
		{LevelAt{Work: WorkScreen, Level: LevelEvery}, looks(y, y, y, y, y, y, y, y, y), false},       // nine looks
		{LevelAt{Work: WorkScreen, Level: LevelEvery}, looks(y, y, y, y, y, y, y, y, y, y), true},     // ten in a row
		{LevelAt{Work: WorkScreen, Level: LevelEvery}, looks(y, y, y, y, y, y, y, y, y, n), false},    // the last missed
		{LevelAt{Work: WorkScreen, Level: LevelEvery}, looks(n, y, y, y, y, y, y, y, y, y, y), true},  // an older miss falls out of the last ten
		{LevelAt{Work: WorkDaemon, Level: LevelOneInFive}, looks(y, y, y, y, y, y, y, y, y, y), true}, // down to never
		{LevelAt{Work: WorkScreen, Level: LevelNever}, looks(y, y, y, y, y, y, y, y, y, y), false},    // nowhere further
		{LevelAt{Work: WorkContract, Level: LevelEvery}, looks(y, y, y, y, y, y, y, y, y, y), false},  // the floor
	} {
		if got, _ := LearnedEnough(c.l, c.rec); got != c.want {
			t.Errorf("%+v %v: %v, want %v", c.l, c.rec, got, c.want)
		}
	}
	if LevelDown(LevelEvery) != LevelOneInTwo || LevelDown(LevelOneInTwo) != LevelOneInThree || LevelDown(LevelOneInThree) != LevelOneInFive ||
		LevelDown(LevelOneInFive) != LevelNever || LevelDown(LevelNever) != "" {
		t.Fatal("the steps")
	}
}

// The kind of work is read from each changed file; a task of several kinds is each of them, only deletions outside contract are docs, no
// files is no kind (every), and the strictest
// level among them is the one that counts.
func TestTheKindOfWorkIsReadFromTheChangedFiles(t *testing.T) {
	for p, want := range map[string]string{
		"contract/spec/agents.json":             WorkContract,
		"go/kit/store/migrations/0117_x.sql":    WorkContract,
		"go/kit/store/migrations/manifest.json": WorkContract,
		"tools/store_schema.sql":                WorkContract,
		"apps/web/src/settings/AgentPage.tsx":   WorkScreen,
		"packages/app-data/src/read.ts":         WorkScreen,
		"go/data/db/reviewlevel.go":             WorkDaemon,
		"tools/check.sh":                        WorkDaemon,
		"docs/design/turns.md":                  WorkDocs,
		"AGENTS.md":                             WorkDocs,
		"go/README.md":                          WorkDocs,
	} {
		if got := WorkOf(p); got != want {
			t.Errorf("%s: %s, want %s", p, got, want)
		}
	}
	f := func(p string, deleted bool) gitrun.TaskFile { return gitrun.TaskFile{Path: p, Deleted: deleted} }
	for _, c := range []struct {
		files []gitrun.TaskFile
		want  []string
	}{
		{nil, nil},
		{[]gitrun.TaskFile{f("docs/a.md", false), f("contract/x.json", false), f("apps/a.tsx", false)}, []string{WorkContract, WorkScreen, WorkDocs}},
		{[]gitrun.TaskFile{f("go/a.go", true), f("apps/a.tsx", true)}, []string{WorkDocs}},
		{[]gitrun.TaskFile{f("go/a.go", true), f("go/b.go", false)}, []string{WorkDaemon}},
		{[]gitrun.TaskFile{f("contract/x.json", true)}, []string{WorkContract}}, // deleting the contract is no cleanup
	} {
		if got := WorksOf(c.files); !slices.Equal(got, c.want) {
			t.Errorf("%v: %v, want %v", c.files, got, c.want)
		}
	}
	at := func(level string) LevelAt { return LevelAt{Work: level, Level: level} }
	if Strictest(nil).Level != LevelEvery {
		t.Fatal("no kinds is not every")
	}
	if got := Strictest([]LevelAt{at(LevelNever), at(LevelOneInThree), at(LevelOneInFive)}); got.Level != LevelOneInThree {
		t.Fatalf("strictest %+v", got)
	}
}
