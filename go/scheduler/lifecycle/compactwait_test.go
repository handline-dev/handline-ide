package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
)

func setConfig(t *testing.T, g *termMoveRig, yaml string) {
	t.Helper()
	path := paths.ConfigFile(g.m.Home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

// doingTask gives agent a task it holds and is doing.
func doingTask(t *testing.T, g *termMoveRig, agent string) int64 {
	t.Helper()
	tid, err := db.NewTask(g.m.Q, "job", agent, "boss", 0, db.DefaultPriority, "", 0, "")
	if err == nil {
		_, err = db.SetTaskQueued(g.m.Q, tid, actor.OwnerKey)
	}
	if err == nil {
		err = db.SetCurrentTask(g.m.Q, agent, tid, true)
	}
	if err == nil {
		_, err = db.AddEvent(g.m.Q, agent, "task_begin", db.KV{K: "task_id", V: tid})
	}
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := db.DoingTask(g.m.Q, agent); got != tid {
		t.Fatalf("not doing its task: %d", got)
	}
	return tid
}

func TestACompactDeferredBehindATurnHoldsMessagesUntilItRunsOnlyWithTheSetting(t *testing.T) {
	for _, on := range []bool{false, true} {
		g := newTermMoveRig(t)
		if on {
			setConfig(t, g, "compact_at_task_end: true\n")
		}
		g.run.status = "busy"
		if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, ""); !ok || err != nil {
			t.Fatal(ok, err)
		}
		late := g.m.now() + db.NoteWaitStaleS + 1
		if held, _ := db.RestartHeld(g.m.Q, "a", late); held != on {
			t.Fatalf("on %v: held past the ceiling %v", on, held)
		}
		if since, _ := db.CompactDueSince(g.m.Q, "a"); since == 0 {
			t.Fatalf("on %v: the deferred compact keeps no time", on)
		}
		g.run.status = "idle"
		if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, ""); !ok || err != nil {
			t.Fatal(ok, err)
		}
		if held, _ := db.RestartHeld(g.m.Q, "a", late); held {
			t.Fatalf("on %v: held after the compact ran", on)
		}
		if since, _ := db.CompactDueSince(g.m.Q, "a"); since != 0 {
			t.Fatalf("on %v: the due mark outlived the compact", on)
		}
	}
}

func TestADueCompactWaitsForTheTaskInProgressAndLetsMessagesInBetweenTurns(t *testing.T) {
	g := newTermMoveRig(t)
	tid := doingTask(t, g, "a")
	if waits, err := g.v.CompactWaits("a"); waits || err != nil {
		t.Fatalf("waits with the setting off: %v %v", waits, err)
	}
	setConfig(t, g, "compact_at_task_end: true\n")
	late := g.m.now() + db.NoteWaitStaleS + 1
	g.run.status = "busy"
	if waits, err := g.v.CompactWaits("a"); !waits || err != nil {
		t.Fatalf("mid-turn: %v %v", waits, err)
	}
	if held, _ := db.RestartHeld(g.m.Q, "a", late); !held {
		t.Fatal("a running turn is typed into while its compact is due")
	}
	g.run.status = "idle"
	if waits, _ := g.v.CompactWaits("a"); !waits {
		t.Fatal("compacted at a turn's end while its task is in progress")
	}
	if held, _ := db.RestartHeld(g.m.Q, "a", g.m.now()); held {
		t.Fatal("an ended turn mid-task does not get what it may wait on")
	}
	if since, _ := db.CompactDueSince(g.m.Q, "a"); since == 0 {
		t.Fatal("letting messages in forgot the compact is due")
	}
	if _, err := db.MarkTaskDone(g.m.Q, tid, "abc123", ""); err != nil {
		t.Fatal(err)
	}
	if waits, _ := g.v.CompactWaits("a"); waits {
		t.Fatal("still waits after its task closed")
	}
}

func TestCompactNowStopsTheTurnUnderWayOnlyWithTheSetting(t *testing.T) {
	for _, on := range []bool{false, true} {
		g := newTermMoveRig(t)
		if on {
			setConfig(t, g, "compact_now_stops_turn: true\n")
		}
		db.SetAgentState(g.m.Q, "a", "RUNNING", 0, "")
		g.run.status = "busy"
		stopped := 0
		ok, said, err := g.m.Compact("a", SessionStatus{SessionID: "sess-1"}, "boss", func() error { stopped++; return nil })
		if err != nil || !ok {
			t.Fatalf("on %v: %v %q %v", on, ok, said, err)
		}
		if want := map[bool]int{false: 0, true: 1}[on]; stopped != want {
			t.Fatalf("on %v: stopped %d times: %q", on, stopped, said)
		}
		if _, p, asked, _ := db.LastEvent(g.m.Q, "a", "compact_asked"); !asked || p["by"] != "boss" {
			t.Fatalf("on %v: the ask is not kept: %v %v", on, asked, p)
		}
	}
}

// A turn that has ended with background work behind its prompt is not held without bound: the work may never end.
func TestAnIdleAgentWithBackgroundWorkIsNotHeldWithoutBound(t *testing.T) {
	g := newTermMoveRig(t)
	setConfig(t, g, "compact_at_task_end: true\n")
	g.run.status, g.run.bgIdle = "busy", true
	late := g.m.now() + db.NoteWaitStaleS + 1
	if ok, err := g.v.PerformHandoff("a", HandoffTrigger{Kind: "context", Reason: "full"}, ""); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if held, _ := db.RestartHeld(g.m.Q, "a", late); held {
		t.Fatal("background work holds deliveries past the ceiling")
	}
	doingTask(t, g, "a")
	if waits, _ := g.v.CompactWaits("a"); !waits {
		t.Fatal("does not wait for its task")
	}
	if held, _ := db.RestartHeld(g.m.Q, "a", g.m.now()); held {
		t.Fatal("an agent idle behind background work mid-task does not get what it waits on")
	}
}
