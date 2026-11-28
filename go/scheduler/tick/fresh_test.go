package tick

import (
	"testing"

	"github.com/handline/handline/go/data/db"
)

// taskOf writes a task of owner's under parent (0: none), done at doneAt (0: open), and answers its id.
func taskOf(t *testing.T, tk *Tick, owner string, doneAt, parent int64) int64 {
	t.Helper()
	var p any
	if parent != 0 {
		p = parent
	}
	var res interface{ LastInsertId() (int64, error) }
	var err error
	if doneAt > 0 {
		res, err = tk.Q.Exec("INSERT INTO tasks(title, owner, opened_by, created_at, done_evidence, done_at, parent_task) VALUES('t', ?, 'lead', 1, 'ok', ?, ?)", owner, doneAt, p)
	} else {
		res, err = tk.Q.Exec("INSERT INTO tasks(title, owner, opened_by, created_at, parent_task) VALUES('t', ?, 'lead', 1, ?)", owner, p)
	}
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// queued puts a delivery of task tid in owner's queue.
func queued(t *testing.T, tk *Tick, owner string, tid int64) {
	t.Helper()
	rid, err := db.AddRequest(tk.Q, db.NewRequest{Requester: "lead", Target: owner, Body: "do it", Kind: "task"})
	if err == nil {
		err = db.LinkRequest(tk.Q, tid, rid)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func agentRow(t *testing.T, tk *Tick, agent string) {
	t.Helper()
	if err := db.SetAgentState(tk.Q, agent, "IDLE", 0, ""); err != nil {
		t.Fatal(err)
	}
}

func freshHeld(tk *Tick) (bool, error) {
	return FreshHeld(tk.Q, tk.Config, "a", tk.Lifecycle.TriggersApply)
}

func freshWorld(t *testing.T) (*Tick, *world) {
	tk, w := newWorld(t)
	w.cfg = map[string]any{FreshKey: true}
	w.performOK = true
	agentRow(t, tk, "a")
	return tk, w
}

func TestADoneWithAnUnrelatedTaskQueuedStartsFreshAndTheTaskWaitsForTheNewSession(t *testing.T) {
	tk, w := freshWorld(t)
	taskOf(t, tk, "a", 9_000, 0)
	queued(t, tk, "a", taskOf(t, tk, "a", 0, 0))
	if held, err := freshHeld(tk); err != nil || !held {
		t.Fatalf("the delivery is not held for the fresh session: %v %v", held, err)
	}
	if actions := supervise(tk, "a"); !w.did("handoff a task_done") || !has(actions, "a: handoff (task_done)") {
		t.Fatalf("%v %v", w.calls, actions)
	}
	// The new session began after the done: the task is no longer held, so it goes to that session.
	if _, err := tk.Q.Exec("INSERT INTO events(ts, agent, kind) VALUES(9500, 'a', 'SessionStart')"); err != nil {
		t.Fatal(err)
	}
	if held, _ := freshHeld(tk); held {
		t.Fatal("still held once the new session began")
	}
}

func TestAFreshStartIsOwedOnlyBetweenUnrelatedTasksWithNothingInProgress(t *testing.T) {
	cases := map[string]struct {
		set  func(tk *Tick, w *world)
		want bool
	}{
		"switch off": {func(tk *Tick, w *world) { w.cfg = map[string]any{} }, false},
		"off for the agent, on globally": {func(tk *Tick, w *world) {
			w.cfg = map[string]any{FreshKey: false}
			w.global = map[string]any{FreshKey: true}
		}, false},
		"on globally": {func(tk *Tick, w *world) {
			w.cfg = map[string]any{}
			w.global = map[string]any{FreshKey: true}
		}, true},
		"nothing queued": {func(tk *Tick, w *world) {
			if _, err := tk.Q.Exec("DELETE FROM requests"); err != nil {
				t.Fatal(err)
			}
		}, false},
		"a task in progress": {func(tk *Tick, w *world) {
			if err := db.SetCurrentTask(tk.Q, "a", taskOf(t, tk, "a", 0, 0), true); err != nil {
				t.Fatal(err)
			}
		}, false},
		"a current task that only waits on a blocker": {func(tk *Tick, w *world) {
			cur, blocker := taskOf(t, tk, "a", 0, 0), taskOf(t, tk, "b", 0, 0)
			if _, err := tk.Q.Exec("INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(?, ?, 1)", cur, blocker); err != nil {
				t.Fatal(err)
			}
			if err := db.SetCurrentTask(tk.Q, "a", cur, true); err != nil {
				t.Fatal(err)
			}
		}, true},
		"a session began after the done": {func(tk *Tick, w *world) {
			if _, err := tk.Q.Exec("INSERT INTO events(ts, agent, kind) VALUES(9500, 'a', 'SessionStart')"); err != nil {
				t.Fatal(err)
			}
		}, false},
		"a handoff followed the done": {func(tk *Tick, w *world) {
			if err := startedHandoff(tk, "a", 9_500); err != nil {
				t.Fatal(err)
			}
		}, false},
		"another agent's task is in progress": {func(tk *Tick, w *world) {
			if err := db.SetCurrentTask(tk.Q, "b", taskOf(t, tk, "b", 0, 0), true); err != nil {
				t.Fatal(err)
			}
		}, true},
	}
	for name, c := range cases {
		tk, w := freshWorld(t)
		taskOf(t, tk, "a", 9_000, 0)
		queued(t, tk, "a", taskOf(t, tk, "a", 0, 0))
		c.set(tk, w)
		held, err := freshHeld(tk)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		supervise(tk, "a")
		if fired := w.did("handoff a task_done"); fired != c.want || held != c.want {
			t.Errorf("%s: handed off %v, held %v, want %v", name, fired, held, c.want)
		}
	}
}

func TestANextTaskRelatedToTheOneClosedGoesOnInTheSameSession(t *testing.T) {
	cases := map[string]func(tk *Tick) (done, next int64){
		"same parent": func(tk *Tick) (int64, int64) {
			story := taskOf(t, tk, "lead", 0, 0)
			return taskOf(t, tk, "a", 9_000, story), taskOf(t, tk, "a", 0, story)
		},
		"the next is the parent": func(tk *Tick) (int64, int64) {
			parent := taskOf(t, tk, "a", 0, 0)
			return taskOf(t, tk, "a", 9_000, parent), parent
		},
		"the next is a child": func(tk *Tick) (int64, int64) {
			done := taskOf(t, tk, "a", 9_000, 0)
			return done, taskOf(t, tk, "a", 0, done)
		},
		"begun in this session": func(tk *Tick) (int64, int64) {
			next := taskOf(t, tk, "a", 0, 0)
			if _, err := tk.Q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(8000, 'a', 'task_begin', json_object('task_id', ?))", next); err != nil {
				t.Fatal(err)
			}
			return taskOf(t, tk, "a", 9_000, 0), next
		},
	}
	for name, mk := range cases {
		tk, w := freshWorld(t)
		_, next := mk(tk)
		queued(t, tk, "a", next)
		if held, _ := freshHeld(tk); held {
			t.Errorf("%s: held", name)
		}
		if supervise(tk, "a"); w.did("handoff a task_done") {
			t.Errorf("%s: handed off", name)
		}
	}
}

func startedHandoff(tk *Tick, agent string, at int64) error {
	_, err := tk.Q.Exec("INSERT INTO handoff(id, agent, session_id, step, state, trigger_kind, updated, deadline) VALUES('h1', ?, 's', 'released', 'done', 'context', ?, ?)", agent, at, at+300)
	return err
}

func handedOn(t *testing.T, tk *Tick, from string, tid, at int64) {
	t.Helper()
	if _, err := tk.Q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, ?, 'task_handoff', json_object('task_id', ?, 'from', ?, 'to', 'someone.else'))", at, from, tid, from); err != nil {
		t.Fatal(err)
	}
}

func TestATaskHandedOnCountsAsClosedButAnotherAgentsHandoffDoesNot(t *testing.T) {
	tk, w := freshWorld(t)
	handedOn(t, tk, "a", taskOf(t, tk, "someone.else", 0, 0), 9_000)
	queued(t, tk, "a", taskOf(t, tk, "a", 0, 0))
	if supervise(tk, "a"); !w.did("handoff a task_done") {
		t.Fatalf("a task ended by a handoff is a closed task: %v", w.calls)
	}

	tk, w = freshWorld(t)
	handedOn(t, tk, "b", taskOf(t, tk, "someone.else", 0, 0), 9_000)
	queued(t, tk, "a", taskOf(t, tk, "a", 0, 0))
	if supervise(tk, "a"); w.did("handoff a task_done") {
		t.Fatal("handed off for another agent's task")
	}
}

func TestTheHoldLiftsWhileTheTickCannotHandTheAgentOffAndABadHandoffRowFailsNothing(t *testing.T) {
	cases := map[string]func(tk *Tick, w *world){
		"no trigger applies (degraded, parked, no terminal)": func(tk *Tick, w *world) { w.triggers = false },
		"stalled": func(tk *Tick, w *world) {
			if err := db.SetAgentState(tk.Q, "a", "STALLED", 0, ""); err != nil {
				t.Fatal(err)
			}
		},
		"a context compact is due": func(tk *Tick, w *world) {
			if err := db.MarkCompactDue(tk.Q, "a", 9_600, false); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, set := range cases {
		tk, w := freshWorld(t)
		taskOf(t, tk, "a", 9_000, 0)
		queued(t, tk, "a", taskOf(t, tk, "a", 0, 0))
		set(tk, w)
		if held, err := freshHeld(tk); held || err != nil {
			t.Errorf("%s: held %v, %v", name, held, err)
		}
	}
	tk, _ := freshWorld(t)
	queued(t, tk, "a", taskOf(t, tk, "a", 0, 0))
	if _, err := tk.Q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(9000, 'a', 'task_handoff', json_object('from', 'a')), " +
		"(9100, 'a', 'task_handoff', 'not json')"); err != nil {
		t.Fatal(err)
	}
	if held, err := freshHeld(tk); !held || err != nil {
		t.Fatalf("a handoff row with no task id is a closed task of no parent: %v %v", held, err)
	}
}
