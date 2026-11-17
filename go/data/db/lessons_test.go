package db

import (
	"errors"
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
