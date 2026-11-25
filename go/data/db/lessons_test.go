package db

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Lessons keep who and when: an add, a merge and a removal each leave the rows, and only live ones reach the prompt.
func TestLessonsAreAddedMergedAndRemovedKeepingEveryRow(t *testing.T) {
	q := open(t)
	tid := seedTask(t, q, "a1", "p1")
	one, err := AddLesson(q, "a1", "say what is left out", tid, "a1")
	if err != nil {
		t.Fatal(err)
	}
	two, _ := AddLesson(q, "a1", "say what was left out", 0, "a1")
	three, _ := AddLesson(q, "a1", "shots in both themes", 0, "a1")
	if _, err := AddLesson(q, "a2", "someone else's", 0, "a2"); err != nil {
		t.Fatal(err)
	}
	var events int
	_ = q.QueryRow("SELECT COUNT(*) FROM events WHERE kind = ?", LessonKind).Scan(&events)
	if events != 1 {
		t.Fatalf("%d lesson events, want 1 (only a lesson with a task writes one)", events)
	}
	merged, err := MergeLessons(q, "a1", []int64{one, two}, "say what is left out, and why", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MergeLessons(q, "a1", []int64{one, three}, "again", "a1"); err == nil {
		t.Fatal("merged a lesson merged already")
	}
	if ok, _ := RemoveLesson(q, three, "p1"); !ok {
		t.Fatal("not removed")
	}
	live, _ := LiveLessons(q, "a1")
	if len(live) != 1 || live[0].ID != merged || live[0].Task != tid {
		t.Fatalf("live %+v, want only the merge, carrying T%d", live, tid)
	}
	var rows, into int64
	var by string
	_ = q.QueryRow("SELECT COUNT(*) FROM lessons WHERE agent = 'a1'").Scan(&rows)
	_ = q.QueryRow("SELECT merged_into, removed_by FROM lessons WHERE id = ?", one).Scan(&into, &by)
	if rows != 4 || into != merged || by != "a1" {
		t.Fatalf("rows %d, merged_into %d by %q", rows, into, by)
	}
	if s := LessonsSection(live); !strings.Contains(s, "- say what is left out, and why (T") {
		t.Fatalf("section %q", s)
	}
}

// An add past the cap is refused; a merge at the cap goes through, since it makes room.
func TestLessonsStopAtTheCapAndAMergeMakesRoom(t *testing.T) {
	q := open(t)
	long := strings.Repeat("x", 490)
	var ids []int64
	for {
		id, err := AddLesson(q, "a1", long, 0, "a1")
		if errors.Is(err, ErrLessonsFull) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	live, _ := LiveLessons(q, "a1")
	if n := len(LessonsSection(live)); n > LessonsCap || n+len(long)+3 <= LessonsCap {
		t.Fatalf("refused at %d bytes, cap %d", n, LessonsCap)
	}
	if _, err := MergeLessons(q, "a1", ids[:2], long, "a1"); err != nil {
		t.Fatalf("a merge at the cap: %v", err)
	}
	if _, err := AddLesson(q, "a1", long, 0, "a1"); err != nil {
		t.Fatalf("an add after the merge: %v", err)
	}
}

// A person's rules are capped per area, and a set of revisions is checked once, on what it leaves.
func TestAPersonsRulesAreCappedPerAreaAndRevisedAtOnce(t *testing.T) {
	q := open(t)
	long := strings.Repeat("x", 5000)
	for range 2 {
		if _, err := AddLesson(q, "p1", "Screen: "+long, 0, "p1"); err != nil {
			t.Fatal(err)
		}
		if _, err := AddLesson(q, "p1", "Work: "+long, 0, "p1"); err != nil {
			t.Fatal("each area has a cap of its own", err)
		}
	}
	if _, err := AddLesson(q, "p1", "Screen: "+long+long, 0, "p1"); !errors.Is(err, ErrLessonsFull) {
		t.Fatal("past the screen cap must be refused", err)
	}
	live, _ := LiveLessons(q, "p1")
	// Replacing both screen rules with one longer one fits once the two go, whatever the order.
	if err := ReviseLessons(q, "p1", []Revision{{Text: "Screen: " + long + long}, {IDs: []int64{live[0].ID, live[2].ID}, Text: "Screen: short", Task: 7}}, "f"); err != nil {
		t.Fatal(err)
	}
	if err := ReviseLessons(q, "p1", []Revision{{Text: "Screen: " + long + long}}, "f"); !errors.Is(err, ErrLessonsFull) {
		t.Fatal(err)
	}
	if err := ReviseLessons(q, "p1", []Revision{{IDs: []int64{live[1].ID}, Text: "a"}, {IDs: []int64{live[1].ID}, Text: "b"}}, "f"); err == nil {
		t.Fatal("an id in two revisions must be refused")
	}
}

// A launch prompt shows each area of a person's rules up to its cap, the most said first, in the order they were kept.
func TestALaunchShowsAPersonsMostSaidRulesPerArea(t *testing.T) {
	q := open(t)
	long := strings.Repeat("y", 150)
	for i := range 60 {
		_, _ = AddLesson(q, "p1", fmt.Sprintf("Work: %s%d (said %d times)", long, i, i+2), 0, "p1")
	}
	_, _ = AddLesson(q, "p1", "Screen: no hints", 0, "p1")
	_, _ = AddLesson(q, "p1", "Work: by hand", 0, "p1")
	live, _ := LiveLessons(q, "p1")
	s := PromptSection("p1", live)
	if !strings.Contains(s, "- by hand\n") {
		t.Fatal("a rule a person wrote by hand must show before the fold's most said:\n" + s)
	}
	if !strings.Contains(s, long+"59\n") || strings.Contains(s, long+"0\n") || !strings.Contains(s, "## Screen\n- no hints") || strings.Contains(s, "- Work: ") || strings.Contains(s, "(said") {
		t.Fatal("the most said must show, only as its rule, and the least said fall past the cap:\n" + s)
	}
	if len(s) > ShowCap[AreaWork]+ShowCap[AreaScreen]+200 {
		t.Fatal(len(s))
	}
	if PromptSection("a1", []Lesson{{Text: "x"}}) != LessonsSection([]Lesson{{Text: "x"}}) {
		t.Fatal("an agent's own lessons show whole")
	}
}
