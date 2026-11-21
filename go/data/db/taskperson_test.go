package db

import "testing"

// A task's person is the person it answers to (its reporter, else its opener), else its nearest parent's, else whose request it serves,
// else the owner; a removed person is passed over.
func TestTaskPersonIsWhomTheTaskAnswersTo(t *testing.T) {
	q := open(t)
	for _, s := range []string{
		"INSERT INTO people(id, display_name, role, created_at) VALUES(2, 'Kim', 'member', 0), (3, 'Lee', 'member', 0)",
		"INSERT INTO requests(id, ref, ts, requester, target, body, state, edge_ok) VALUES(9, 'aaaaaaaaaaa9', 1, 'p3', 'a1', 'go', 'landed', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'opened by Kim', 'a1', 'p2', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, reporter, created_at) VALUES(2, 'reports to Lee', 'a1', 'p2', 'p3', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at) VALUES(3, 'under Kim''s', 'a2', 'a1', 1, 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, request, created_at) VALUES(4, 'serves Lee''s request', 'a2', 'a1', 9, 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(5, 'an agent''s own', 'a2', 'a1', 1)",
		"INSERT INTO people(id, display_name, role, created_at, removed_at) VALUES(4, 'Gone', 'member', 0, 5)",
		"INSERT INTO tasks(id, title, owner, opened_by, reporter, parent_task, created_at) VALUES(6, 'reports to Gone', 'a2', 'p4', 'p4', 1, 1)",
	} {
		if _, err := q.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	for tid, want := range map[int64]string{1: "p2", 2: "p3", 3: "p2", 4: "p3", 5: "p1", 6: "p2"} {
		if got, err := TaskPerson(q, tid); err != nil || got != want {
			t.Errorf("T%d: %q %v, want %s", tid, got, err, want)
		}
	}
}
