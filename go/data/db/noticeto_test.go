package db

import (
	"testing"

	"github.com/handline/handline/go/kit/actor"
)

func TestANoticeGoesToTheAgentThatOpenedTheTaskElseTheFounder(t *testing.T) {
	for _, c := range []struct {
		name, opener, about, want string
	}{
		{"an agent opened it", "amy", "bob", "amy"},
		{"the agent it is about opened it", "bob", "bob", actor.OwnerKey},
		{"the founder opened it", actor.OwnerKey, "bob", actor.OwnerKey},
		{"nobody is on record", "", "bob", actor.OwnerKey},
	} {
		if got := NoticeTo(&Task{OpenedBy: c.opener}, c.about); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if got := NoticeTo(nil, "bob"); got != actor.OwnerKey {
		t.Errorf("no task: %q", got)
	}
}

func TestANoticeAboutAnAgentGoesToTheOpenerOfTheOldestTaskWaitingForIt(t *testing.T) {
	q := open(t)
	if to, tid, err := AgentNoticeTo(q, "bob"); err != nil || to != actor.OwnerKey || tid != 0 {
		t.Fatalf("nothing waits for bob: the founder, %q %d %v", to, tid, err)
	}
	for _, s := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'done', 'bob', 'cat', 1), (2, 'old', 'bob', 'amy', 2), (3, 'new', 'bob', 'cat', 3)",
		"UPDATE tasks SET done_evidence = 'x' WHERE id = 1",
	} {
		if _, err := q.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	if to, tid, err := AgentNoticeTo(q, "bob"); err != nil || to != "amy" || tid != 2 {
		t.Fatalf("T2 is the oldest still open: %q %d %v", to, tid, err)
	}
}

func TestAWatchIsDueOnceAnHourOnAndOnlyWhenTheTaskHasNotMoved(t *testing.T) {
	q := open(t)
	if _, err := q.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'bob', 'amy', 1), (2, 'b', 'bob', 'amy', 1)"); err != nil {
		t.Fatal(err)
	}
	for _, tid := range []int64{1, 2} {
		if err := WatchNotice(q, tid, "amy", "stuck", 1000); err != nil {
			t.Fatal(err)
		}
	}
	if err := WatchNotice(q, 1, "amy", "stuck", 2000); err != nil { // told again: the hour still counts from the first
		t.Fatal(err)
	}
	// T2 moves within the hour: a comment.
	if _, err := q.Exec("INSERT INTO task_memos(task_id, author, body, created_at) VALUES(2, 'amy', 'on it', 1500)"); err != nil {
		t.Fatal(err)
	}
	if due, _ := DueNoticeWatches(q, 1000+NoticeWatchS-1); len(due) != 0 {
		t.Fatalf("before the hour: %+v", due)
	}
	due, err := DueNoticeWatches(q, 1000+NoticeWatchS)
	if err != nil || len(due) != 1 || due[0].Task != 1 || due[0].To != "amy" || due[0].Text != "stuck" {
		t.Fatalf("T1 has not moved, T2 has: %+v %v", due, err)
	}
	if again, _ := DueNoticeWatches(q, 1000+2*NoticeWatchS); len(again) != 0 {
		t.Fatalf("each watch is due once: %+v", again)
	}
}

func TestATaskMovesByAnEventOrACommentFromTheSecondAfterButNotByTheBoardTelling(t *testing.T) {
	q := open(t)
	if _, err := q.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'bob', 'amy', 1)"); err != nil {
		t.Fatal(err)
	}
	moved := func() bool {
		m, err := TaskMovedSince(q, 1, 1000)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, s := range []string{
		`INSERT INTO events(ts, agent, kind, payload) VALUES(1000, 'bob', 'task_begin', '{"task_id": 1}')`,
		`INSERT INTO events(ts, agent, kind, payload) VALUES(2000, 'bob', 'task_timeout_exceeded', '{"task_id": 1}')`,
		`INSERT INTO events(ts, agent, kind, payload) VALUES(2000, 'bob', 'task_begin', '{"task_id": 2}')`,
	} {
		if _, err := q.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	if moved() {
		t.Fatal("an event in the notice's own second, the board telling, or another task's event is no move")
	}
	if _, err := q.Exec(`INSERT INTO events(ts, agent, kind, payload) VALUES(1001, 'bob', 'task_handoff', '{"task_id": 1}')`); err != nil {
		t.Fatal(err)
	}
	if !moved() {
		t.Fatal("a handoff the second after is a move")
	}
}

func TestAnAgentNoticeIsOverWhenTheAgentActsNotWhenTheBoardWritesAboutIt(t *testing.T) {
	q := open(t)
	n := FounderNotice{Agent: "bob", Reason: "stale_background", Text: "stale", At: 1000}
	if err := NoteFounderNotice(q, n); err != nil {
		t.Fatal(err)
	}
	over := func() bool {
		o, err := FounderNoticeOver(q, n)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	if _, err := q.Exec(`INSERT INTO events(ts, agent, kind, payload) VALUES(1005, 'bob', 'background_checked', '{}')`); err != nil {
		t.Fatal(err)
	}
	if over() {
		t.Fatal("the board's own event about bob is not bob acting")
	}
	if _, err := q.Exec("INSERT INTO requests(ts, requester, target, body, state) VALUES(1000, 'bob', 'amy', 'same second', 'landed')"); err != nil {
		t.Fatal(err)
	}
	if over() {
		t.Fatal("a message in the notice's own second is not after it")
	}
	if err := ClearOverFounderNotices(q); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Meta(q, FounderNoticePrefix+"bob:stale_background"); !ok {
		t.Fatal("a notice not over is kept")
	}
	if _, err := q.Exec("INSERT INTO requests(ts, requester, target, body, state) VALUES(1001, 'bob', 'amy', 'back', 'landed')"); err != nil {
		t.Fatal(err)
	}
	if !over() {
		t.Fatal("a message of bob's after it ends it")
	}
	if err := ClearOverFounderNotices(q); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Meta(q, FounderNoticePrefix+"bob:stale_background"); ok {
		t.Fatal("a notice that is over is dropped, so it cannot come back")
	}
}
