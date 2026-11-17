package db

import (
	"fmt"
	"testing"
)

// The record is the first verdict by a person on each task the agent made Ready, after its level was set: an automatic OK is no look, a later
// verdict on the same task is no second look, and a level change starts it again.
func TestTheReviewRecordCountsFirstLooksAtTheLevelNow(t *testing.T) {
	q := open(t)
	readyBy := func(agent string) int64 {
		t.Helper()
		tid := seedTask(t, q, agent, "p1")
		if err := AddReady(q, tid, agent, "a9", 0); err != nil {
			t.Fatal(err)
		}
		return tid
	}
	task := func() int64 { return readyBy("a1") }
	verdict := func(tid int64, by, v string) {
		t.Helper()
		if err := AddVerdict(q, tid, by, v, 0); err != nil {
			t.Fatal(err)
		}
	}
	old := task()
	verdict(old, "p1", VerdictRedo)
	if err := SetReviewLevel(q, "a1", LevelOneInThree, "p1", "set", 0); err != nil {
		t.Fatal(err)
	}
	verdict(old, "p1", VerdictOK) // its first look was before the level
	one, two, three, other := task(), task(), task(), readyBy("a2")
	verdict(one, "p1", VerdictOK)
	verdict(two, "p1", VerdictRedo)
	verdict(two, "p1", VerdictOK)
	verdict(three, AutoVerdictBy, VerdictOK)
	verdict(other, "p1", VerdictOK)
	l, _ := ReviewLevel(q, "a1")
	rec, err := ReviewRecord(q, "a1", l)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(rec); got != fmt.Sprint([]Look{{one, true, 1000}, {two, false, 1000}}) {
		t.Fatalf("record %s", got)
	}
	if err := SetReviewLevel(q, "a1", LevelEvery, "a1", "redo", two); err != nil {
		t.Fatal(err)
	}
	l, _ = ReviewLevel(q, "a1")
	if rec, _ := ReviewRecord(q, "a1", l); len(rec) != 0 {
		t.Fatalf("a new level kept %v", rec)
	}
	if l, _ := ReviewLevel(q, "a2"); l.Level != LevelEvery || l.Row != 0 {
		t.Fatalf("an agent no row set: %+v", l)
	}
}

// Learned enough is 8 of the last 10 OK at first, never at auto.
func TestLearnedEnoughIsEightOfTheLastTen(t *testing.T) {
	looks := func(oks ...bool) []Look {
		var out []Look
		for _, ok := range oks {
			out = append(out, Look{OKFirst: ok})
		}
		return out
	}
	y, n := true, false
	for _, c := range []struct {
		level string
		rec   []Look
		want  bool
	}{
		{LevelEvery, looks(y, y, y, y, y, y, y, y, y), false},         // nine looks
		{LevelEvery, looks(y, y, y, y, y, y, y, y, n, n), true},       // 8 of 10
		{LevelEvery, looks(y, y, y, y, y, y, y, n, n, n), false},      // 7 of 10
		{LevelEvery, looks(n, n, n, y, y, y, y, y, y, y, y, n), true}, // older misses fall out of the last ten
		{LevelOneInThree, looks(y, y, y, y, y, y, y, y, y, y), true},
		{LevelAuto, looks(y, y, y, y, y, y, y, y, y, y), false},
	} {
		if got, _ := LearnedEnough(c.level, c.rec); got != c.want {
			t.Errorf("%s %v: %v", c.level, c.rec, got)
		}
	}
	if LevelUp(LevelAuto) != LevelOneInThree || LevelUp(LevelOneInThree) != LevelEvery || LevelUp(LevelEvery) != LevelEvery ||
		LevelDown(LevelEvery) != LevelOneInThree || LevelDown(LevelAuto) != "" {
		t.Error("the steps")
	}
}
