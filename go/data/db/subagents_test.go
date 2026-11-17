package db

import (
	"strings"
	"testing"
	"time"
)

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
