package tick

import (
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

func stalled(t *testing.T, tk *Tick, agent string) {
	t.Helper()
	if err := db.SetStalled(tk.Q, agent, "start", "3 starts failed", nil); err != nil {
		t.Fatal(err)
	}
}

func stateOf(t *testing.T, tk *Tick, agent string) string {
	t.Helper()
	row, err := db.GetAgentState(tk.Q, agent)
	if err != nil || row == nil {
		t.Fatal(err)
	}
	return row.State
}

func TestAPersonsMessageToAStalledAgentClearsTheStallOnce(t *testing.T) {
	tk, _ := newWorld(t)
	stalled(t, tk, "a")
	if _, err := db.AddRequest(tk.Q, db.NewRequest{Requester: "a9", Target: "a", Body: "x", Kind: "say"}); err != nil {
		t.Fatal(err)
	}
	supervise(tk, "a")
	if s := stateOf(t, tk, "a"); s != states.Stalled {
		t.Fatalf("an agent's message cleared the stall: %s", s)
	}
	if _, err := db.AddRequest(tk.Q, db.NewRequest{Requester: "p1", Target: "a", Body: "try", Kind: "say"}); err != nil {
		t.Fatal(err)
	}
	supervise(tk, "a")
	if s := stateOf(t, tk, "a"); s != states.Queued {
		t.Fatalf("a person's message left it %s", s)
	}
	// stalled again by its next failures: the same message does not clear it twice
	stalled(t, tk, "a")
	supervise(tk, "a")
	if s := stateOf(t, tk, "a"); s != states.Stalled {
		t.Fatalf("one message cleared the stall twice: %s", s)
	}
}

func TestAPersonsCommentForwardedToAStalledAgentClearsTheStall(t *testing.T) {
	tk, _ := newWorld(t)
	if _, err := tk.Q.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'a', 'p1', 1)"); err != nil {
		t.Fatal(err)
	}
	for _, by := range []string{"a9", "p1"} {
		stalled(t, tk, "a")
		rid, err := db.AddRequest(tk.Q, db.NewRequest{Requester: "sys", Target: "a", Body: "comment", Kind: "say"})
		if err != nil {
			t.Fatal(err)
		}
		res, err := tk.Q.Exec("INSERT INTO task_comments(task_id, author, body, created_at) VALUES(1, ?, 'c', 1)", by)
		if err != nil {
			t.Fatal(err)
		}
		mid, _ := res.LastInsertId()
		if _, err := tk.Q.Exec("INSERT INTO comment_tells(comment_id, reader, request_id) VALUES(?, 'a', ?)", mid, rid); err != nil {
			t.Fatal(err)
		}
		supervise(tk, "a")
		want := states.Stalled
		if by == "p1" {
			want = states.Queued
		}
		if s := stateOf(t, tk, "a"); s != want {
			t.Fatalf("a comment by %s left it %s", by, s)
		}
	}
}

func TestAPersonsMessageWrittenBeforeTheStallDoesNotClearIt(t *testing.T) {
	tk, _ := newWorld(t)
	if _, err := db.AddRequest(tk.Q, db.NewRequest{Requester: "p1", Target: "a", Body: "go", Kind: "say"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAgentState(tk.Q, "a", states.Stalled, 0, "3 failed attempts"); err != nil {
		t.Fatal(err)
	}
	supervise(tk, "a")
	if s := stateOf(t, tk, "a"); s != states.Stalled {
		t.Fatalf("the message its failed starts were for cleared the stall: %s", s)
	}
}
