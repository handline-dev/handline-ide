package db

import (
	"testing"
	"time"
)

func TestSubagentCountsRunUntilTheirStopOrTheirSessionsEnd(t *testing.T) {
	q := open(t)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	ev := func(agent, kind, session string, at time.Time) {
		t.Helper()
		if _, err := q.Exec("INSERT INTO events(ts, agent, account, kind, payload) VALUES(?,?,'beta',?,json_object('session_id', ?))", at.Unix(), agent, kind, session); err != nil {
			t.Fatal(err)
		}
	}
	start := func(agent, session string, at time.Time) {
		t.Helper()
		if err := RecordSubagentStart(q, agent, "beta", session, at.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	h := time.Hour
	start("a", "s1", now.Add(-13*h)) // yesterday, still running in a live session
	start("a", "s1", now.Add(-2*h))
	start("a", "s1", now.Add(-h))
	ev("a", "SubagentStop", "s1", now.Add(-h/2))
	start("b", "s2", now.Add(-h)) // its session ended mid-subagent
	ev("b", "SessionEnd", "s2", now.Add(-h/2))
	start("c", "s3", now.Add(-h)) // a new session replaced it
	ev("c", "SessionStart", "s4", now.Add(-h/2))
	start("d", "s5", now.Add(-h)) // the agent's session closed
	if _, err := q.Exec("INSERT INTO agent_state(agent, state, updated, closed_at) VALUES('d', 'idle', 1, ?)", now.Unix()); err != nil {
		t.Fatal(err)
	}

	agents, accounts, err := SubagentCounts(q, now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]SubagentCount{"a": {Running: 2, Today: 2}, "b": {Today: 1}, "c": {Today: 1}, "d": {Today: 1}}
	for name, w := range want {
		if agents[name] != w {
			t.Errorf("%s: %+v, want %+v", name, agents[name], w)
		}
	}
	if got := accounts["beta"]; got != (SubagentCount{Running: 2, Today: 5}) {
		t.Errorf("beta: %+v", got)
	}
}
