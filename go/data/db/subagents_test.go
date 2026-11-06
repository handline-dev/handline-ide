package db

import (
	"strings"
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
		if err := RecordSubagentStart(q, agent, "beta", session, at.Unix(), SubagentMeta{}); err != nil {
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

func TestRunningSubagentsCarryWhatTheCallSaidOldestFirst(t *testing.T) {
	q := open(t)
	now := time.Unix(100000, 0)
	for i, m := range []SubagentMeta{{Description: "review", Type: "general-purpose"}, {Type: "Explore", Background: true}, {Description: "done"}} {
		if err := RecordSubagentStart(q, "a", "", "s1", now.Unix()-int64(100-i), m); err != nil {
			t.Fatal(err)
		}
	}
	q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, 'a', 'SubagentStop', json_object('session_id', 's1'))", now.Unix()-10)
	q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, 'a', 'SubagentStart', json_object('session_id', 's1'))", now.Unix()-5) // from before the meta
	if err := RecordSubagentStart(q, "a", "", "s2", now.Unix()-200, SubagentMeta{Type: "late"}); err != nil {                             // a later row, an earlier start
		t.Fatal(err)
	}
	got, err := RunningSubagents(q, "a", now)
	if err != nil || len(got) != 4 {
		t.Fatalf("%v %+v", err, got)
	}
	if got[0].Type != "late" {
		t.Fatalf("not oldest first: %+v", got)
	}
	got = got[1:]
	if got[0].Type != "Explore" || !got[0].Background || got[1].Description != "done" || got[1].Background || got[2].Description != "" || got[2].Type != "" {
		t.Fatalf("the stop ends the oldest; the rest keep their meta: %+v", got)
	}
	if other, _ := RunningSubagents(q, "b", now); len(other) != 0 {
		t.Fatalf("another agent's: %+v", other)
	}
}

func TestASubagentsLabelIsItsDescriptionElseItsType(t *testing.T) {
	for m, want := range map[SubagentMeta]string{
		{Description: "review  the\ndiff", Type: "general-purpose"}: "review the diff",
		{Description: " ", Type: "Explore"}:                         "Explore",
		{}:                                                          "subagent",
		{Description: strings.Repeat("x", 101)}:                     strings.Repeat("x", 99) + "…",
		{Description: strings.Repeat("x", 100)}:                     strings.Repeat("x", 100),
	} {
		if got := (OpenSubagent{SubagentMeta: m}).Label(); got != want {
			t.Errorf("%+v: %q", m, got)
		}
	}
	if got := (OpenSubagent{SubagentMeta: SubagentMeta{Description: strings.Repeat("é", 150)}}).Label(); len([]rune(got)) != 100 {
		t.Fatal(len([]rune(got)))
	}
}
