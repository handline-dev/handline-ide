package daemon

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// The daemon serves lessons: the founder reads an agent's and removes one.
func TestTheDaemonServesLessons(t *testing.T) {
	d, c := startDoors(t, false)
	id, err := db.AddLesson(d.db, "a1", "show the screen in both themes", 0, "a1")
	if err != nil {
		t.Fatal(err)
	}
	items := call(t, c, "lessons.list", `{"agent":"a1"}`)["result"].(map[string]any)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("listed %v", items)
	}
	if r := call(t, c, "lesson.remove", fmt.Sprintf(`{"id":%d}`, id)); r["result"] == nil {
		t.Fatal(fmt.Sprint(r))
	}
	if live, _ := db.LiveLessons(d.db, "a1"); len(live) != 0 {
		t.Fatalf("still live: %v", live)
	}
}

// The daemon serves review levels: the founder sets one at a kind of work and reads it back with its record.
func TestTheDaemonServesReviewLevels(t *testing.T) {
	_, c := startDoors(t, false)
	if r := call(t, c, "agent.review_set", `{"agent":"a1","work":"screen","level":"never"}`); r["result"] == nil {
		t.Fatal(fmt.Sprint(r))
	}
	got := call(t, c, "agent.review", `{"agent":"a1"}`)["result"].(map[string]any)
	if fmt.Sprint(got["ladder"]) != "[map[level:never looks:0 record:[] suggested:<nil> work:screen]]" {
		t.Fatalf("%v", got)
	}
}
