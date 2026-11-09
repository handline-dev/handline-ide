package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/rowstate"
	"github.com/handline/handline/go/kit/storetest"
)

func TestASwitchIsLiveOnlyUntilWhatEndsItsPhase(t *testing.T) {
	const now = 10_000
	for _, c := range []struct {
		name  string
		phase string
		since int64
		row   db.AgentState
		bound int64 // a session bound at this time
		live  bool
	}{
		{name: "saving on an open session", phase: db.SwitchSaving, since: now - 5000, row: db.AgentState{State: rowstate.Running}, live: true},
		{name: "saving on a closed one", phase: db.SwitchSaving, since: now, row: db.AgentState{State: rowstate.Idle, ClosedAt: closedAt(9000)}},
		{name: "any phase of a stalled agent", phase: db.SwitchSaving, since: now, row: db.AgentState{State: rowstate.Stalled}},
		{name: "starting", phase: db.SwitchStarting, since: now - 10, row: db.AgentState{State: rowstate.Queued}, bound: now - 20, live: true},
		{name: "starting past its session's bind", phase: db.SwitchStarting, since: now - 10, row: db.AgentState{State: rowstate.Queued}, bound: now - 10},
		{name: "switching right at the stale bound", phase: db.SwitchSwitching, since: now - db.NoteWaitStaleS, row: db.AgentState{State: rowstate.Handoff}, live: true},
		{name: "switching past it", phase: db.SwitchSwitching, since: now - db.NoteWaitStaleS - 1, row: db.AgentState{State: rowstate.Handoff}},
		{name: "waiting", phase: db.SwitchWaiting, since: now - 5000, row: db.AgentState{State: rowstate.Parked, WakeAt: wakeAt(now + 1)}, live: true},
		{name: "waiting past its wake", phase: db.SwitchWaiting, since: now - 10, row: db.AgentState{State: rowstate.Parked, WakeAt: wakeAt(now)}},
		{name: "waiting after the park ended", phase: db.SwitchWaiting, since: now - 10, row: db.AgentState{State: rowstate.Queued, WakeAt: wakeAt(now + 1)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { d.Close() })
			if err := db.SetSwitch(d, db.AccountSwitch{Agent: "a", Cause: "usage", From: "alpha", To: "beta", Phase: c.phase, Since: c.since}); err != nil {
				t.Fatal(err)
			}
			if c.bound != 0 {
				if _, err := d.Exec("INSERT INTO session_starts(agent, ts, session_id, prompt, text, cause, resumed) VALUES('a', ?, 's', '', '', 'wake', 0)", c.bound); err != nil {
					t.Fatal(err)
				}
			}
			c.row.Agent = "a"
			got, err := db.LiveSwitch(d, &c.row, now)
			if err != nil || (got != nil) != c.live {
				t.Fatalf("live %v, want %v (%v)", got, c.live, err)
			}
		})
	}
}

func TestNoAgentAndNoRowAreNoSwitch(t *testing.T) {
	d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if got, err := db.LiveSwitch(d, nil, 1); got != nil || err != nil {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := db.LiveSwitch(d, &db.AgentState{Agent: "a"}, 1); got != nil || err != nil {
		t.Fatalf("%v %v", got, err)
	}
}

func TestClearSavingLeavesAPhasePastSaving(t *testing.T) {
	d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	db.SetSwitch(d, db.AccountSwitch{Agent: "s", Phase: db.SwitchSaving})
	db.SetSwitch(d, db.AccountSwitch{Agent: "w", Phase: db.SwitchWaiting})
	for _, a := range []string{"s", "w"} {
		if err := db.ClearSavingSwitch(d, a); err != nil {
			t.Fatal(err)
		}
	}
	if s, _ := db.GetSwitch(d, "s"); s != nil {
		t.Fatalf("saving stayed: %+v", s)
	}
	if s, _ := db.GetSwitch(d, "w"); s == nil {
		t.Fatal("waiting went")
	}
}

func closedAt(at int64) sql.NullInt64 { return sql.NullInt64{Int64: at, Valid: true} }
func wakeAt(at int64) sql.NullInt64   { return sql.NullInt64{Int64: at, Valid: true} }
