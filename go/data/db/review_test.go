package db

import "testing"

// A task serves the founder when it or a task above it serves a founder's request, or is the founder's own. A tree of agents' own tasks does not, and only with the review flow on does serving them make
// a finish Ready; a story's sub-task is Ready either way.
func TestATaskServesTheFounderThroughItsRequestOrAParent(t *testing.T) {
	q := open(t)
	// Opened as task.new opens them, so request is what it sets; 9 is moved under 8 later, 10 and 11 are a parent loop.
	for _, row := range []struct {
		id            int64
		owner, opener string
		reporter      any
		parent        any
	}{{1, "a1", "p1", nil, nil}, {2, "a1", "a1", nil, int64(1)}, {3, "a2", "a2", nil, int64(2)}, {5, "a1", "a1", nil, nil}, {6, "a2", "a1", nil, int64(5)},
		{7, "a1", "a1", "p1", nil}, {8, "p1", "a1", nil, nil}, {9, "a1", "a1", nil, nil}, {10, "a1", "a1", nil, nil}, {11, "a1", "a1", nil, int64(10)}} {
		if _, err := q.Exec("INSERT INTO tasks(id, title, owner, opened_by, reporter, parent_task, created_at) VALUES(?, 't', ?, ?, ?, ?, 1)",
			row.id, row.owner, row.opener, row.reporter, row.parent); err != nil {
			t.Fatal(err)
		}
		if err := SetTaskRequest(q, row.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.Exec("UPDATE tasks SET parent_task = 8 WHERE id = 9; UPDATE tasks SET parent_task = 11 WHERE id = 10"); err != nil {
		t.Fatal(err)
	}
	for tid, want := range map[int64]bool{1: true, 2: true, 3: true, 5: false, 6: false, 7: true, 8: true, 9: true, 10: false} {
		if got, err := ServesFounder(q, tid); err != nil || got != want {
			t.Errorf("T%d serves the founder: %v, want %v (%v)", tid, got, want, err)
		}
	}
	if _, err := AddEvent(q, "a1", "task_new", KV{"task_id", 6}, KV{"draft", true}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		tid  int64
		flow bool
		want bool
	}{{3, false, false}, {3, true, true}, {5, true, false}, {6, false, true}} {
		if got, err := ReadyDue(q, c.tid, c.flow); err != nil || got != c.want {
			t.Errorf("T%d Ready due with the flow %v: %v, want %v (%v)", c.tid, c.flow, got, c.want, err)
		}
	}
	if err := AddVerdict(q, 3, "p1", VerdictOK, 0); err != nil {
		t.Fatal(err)
	}
	if due, _ := ReadyDue(q, 3, true); due {
		t.Error("an OK task is Ready due again")
	}
}
