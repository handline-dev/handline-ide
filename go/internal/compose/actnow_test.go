package compose

import (
	"fmt"
	"testing"
	"time"

	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/pause"
	"github.com/handline/handline/go/scheduler/tick"
	"github.com/handline/handline/go/taskboard/supervisor"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

func viewStatus(t *testing.T, d dbpkg.Q, agent string) (state, reason string) {
	t.Helper()
	d.QueryRow("SELECT json_extract(view, '$.status.state'), json_extract(view, '$.status.reason') FROM agent_view WHERE agent = ?", agent).Scan(&state, &reason)
	return state, reason
}

func launches(d dbpkg.Q, agent string) (n int) {
	d.QueryRow("SELECT COUNT(*) FROM events WHERE agent = ? AND kind = 'launching'", agent).Scan(&n)
	return n
}

// heldTask has alice closed with one thing waiting: a task of bob's for her, which the founder's pause holds (a person's word passes it).
func heldTask(t *testing.T, d dbpkg.Q, alice, bob string) int64 {
	t.Helper()
	if _, err := d.Exec("DELETE FROM requests WHERE target = ?", alice); err != nil {
		t.Fatal(err)
	}
	tid, err := dbpkg.NewTask(d, "the work", alice, bob, 0, dbpkg.DefaultPriority, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	rid, err := dbpkg.AddRequest(d, dbpkg.NewRequest{Requester: bob, Target: alice, Body: "the work", Kind: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec("INSERT INTO task_requests(task_id, request_id) VALUES(?, ?)", tid, rid); err != nil {
		t.Fatal(err)
	}
	return tid
}

// heldAndAsleep starts the loop over alice, closed with a task of bob's the founder's pause holds, and returns once its first tick has run:
// her view says the pause holds her and the loop sleeps PollS. ticks gets one for each later tick.
func heldAndAsleep(t *testing.T) (p *Parts, env *supervisor.Env, alice string, tid int64, ticks chan struct{}) {
	t.Helper()
	p, _, env, _ = wired(t)
	d := p.db
	alice = teamtest.Key(t, p.home, "alice")
	tid = heldTask(t, d, alice, teamtest.Key(t, p.home, "bob"))
	if _, err := pause.Set(d, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	tk := p.Tick(cfgOf(p), TickConfig{Stop: stop, Terminals: true})
	ticks = make(chan struct{}, 8)
	tk.Tick.Passes = append(tk.Tick.Passes, tick.NamedSweep{Name: "count", Run: func(dbpkg.Q, int64) error { ticks <- struct{}{}; return nil }})
	done := tk.Start()
	t.Cleanup(func() { close(stop); <-done })
	select {
	case <-ticks:
	case <-time.After(20 * time.Second):
		t.Fatal("no first tick")
	}
	if n := launches(d, alice); n != 0 {
		t.Fatalf("the pause let alice's wake through: %d launches", n)
	}
	if _, reason := viewStatus(t, d, alice); reason != "paused" {
		t.Fatalf("alice's view does not say the pause holds her: %s", reason)
	}
	return p, env, alice, tid, ticks
}

// startsBetweenTicks is alice's view reading starting at once, and her wake beginning before the loop's next tick.
func startsBetweenTicks(t *testing.T, d dbpkg.Q, alice string, ticks chan struct{}) {
	t.Helper()
	if state, reason := viewStatus(t, d, alice); state != "preparing" {
		t.Fatalf("alice's view at the click: %s %s", state, reason)
	}
	for deadline := time.Now().Add(10 * time.Second); launches(d, alice) == 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("alice was not woken between ticks")
		}
	} // the fake terminals run no agent CLI: that her launch began is the wake
	select {
	case <-ticks:
		t.Fatal("a tick ran: the wake did not come between ticks")
	default:
	}
}

// A resume has the work the pause held go on at once: the held agent's view reads starting at the click, and the loop, asleep until its
// next tick, wakes her now.
func TestAResumeWakesTheAgentThePauseHeldBetweenTicks(t *testing.T) {
	p, env, alice, _, ticks := heldAndAsleep(t)
	if err := pause.Clear(p.db); err != nil {
		t.Fatal(err)
	}
	env.ActNow(alice) // the daemon's resume, for the agents whose views say paused
	startsBetweenTicks(t, p.db, alice, ticks)
}

// So does Run just this (task.send_now), for its one task, with the pause still on.
func TestRunJustThisWakesItsAgentBetweenTicks(t *testing.T) {
	p, env, alice, tid, ticks := heldAndAsleep(t)
	srv := rpc.NewServer(founder{}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	srv.DB, srv.Names = p.db, teamtest.DB(t, p.home)
	taskwrites.Register(srv, p.home, env, nil)
	if r := call(t, srv, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.send_now","params":{"task":%d,"request_id":"s1"}}`, tid))[0]; r["result"] == nil {
		t.Fatalf("%v", r)
	}
	startsBetweenTicks(t, p.db, alice, ticks)
}
