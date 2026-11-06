package db

import "testing"

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// turnOn puts agent on top of task tid, pushed by its owner.
func turnOn(t *testing.T, q Q, tid int64, agent string) {
	t.Helper()
	task, err := GetTask(q, tid)
	if err != nil {
		t.Fatal(err)
	}
	if err := PushTurn(q, tid, Turn{Agent: agent, By: task.Owner, Since: 1}); err != nil {
		t.Fatal(err)
	}
}

// A task owned by a with b on top is b's for the doing slot: b's slot decides whether it may go out and whether it is doing, it is among
// b's open tasks and not a's, and its request is held back from b, not a.
func TestTheAgentOnTopHoldsTheTasksSlot(t *testing.T) {
	q := open(t)
	tid := seedTask(t, q, "a", "x")
	turnOn(t, q, tid, "b")
	_, err := SetTaskQueued(q, tid, "x")
	ok(t, err)
	rid := ask(t, q, "x", "b", "review it", "task")
	ok(t, LinkRequest(q, tid, rid))
	aBusy := seedTask(t, q, "a", "x")
	ok(t, SetCurrentTask(q, "a", aBusy, true))
	holds := func() bool {
		task, _ := GetTask(q, tid)
		ok, err := HoldsCurrent(q, task)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if task, _ := GetTask(q, tid); task.Acting != "b" {
		t.Fatalf("acting %q", task.Acting)
	}
	if !holds() {
		t.Fatal("b is free, so the task may go out though a is busy")
	}
	if mine, _ := OpenTasksOf(q, "b"); len(mine) != 1 || mine[0].ID != tid {
		t.Fatalf("b's open tasks: %+v", mine)
	}
	if theirs, _ := OpenTasksOf(q, "a"); len(theirs) != 1 || theirs[0].ID != aBusy {
		t.Fatalf("a's open tasks: %+v", theirs)
	}
	ok(t, SetCurrentTask(q, "b", tid, true))
	if !holds() {
		t.Fatal("b holds it")
	}
	if doing, _ := IsDoing(q, tid); !doing {
		t.Fatal("the task b holds is doing")
	}
	bBusy := seedTask(t, q, "b", "x")
	ok(t, SetCurrentTask(q, "b", bBusy, true))
	ok(t, SetCurrentTask(q, "a", 0, false))
	if holds() {
		t.Fatal("b holds another task, so this one waits though a is free")
	}
	if doing, _ := IsDoing(q, tid); doing {
		t.Fatal("not the task b holds")
	}
	req, _ := GetRequest(q, rid)
	if held, _ := HeldByHolder(q, "b", req); !held {
		t.Fatal("the request is held back from b, busy with another task")
	}
}

// With the founder on top the task waits on no agent's slot: it may go out and reads doing while its owner holds another task.
func TestTheFoundersTurnHoldsNoSlot(t *testing.T) {
	q := open(t)
	tid := seedTask(t, q, "a", "x")
	_, err := SetTaskQueued(q, tid, "x")
	ok(t, err)
	ok(t, LinkRequest(q, tid, ask(t, q, "x", "a", "work", "task")))
	turnOn(t, q, tid, Human)
	other := seedTask(t, q, "a", "x")
	ok(t, SetCurrentTask(q, "a", other, true))
	task, _ := GetTask(q, tid)
	if ok, _ := HoldsCurrent(q, task); !ok {
		t.Fatal("the founder's turn may go out")
	}
	if doing, _ := IsDoing(q, tid); !doing {
		t.Fatal("the founder's turn reads doing")
	}
}
