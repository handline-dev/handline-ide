package compose

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/scheduler/session"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// The view's background list: the shells only while the footer reports work, the last as many as it counts, and the running subagents,
// oldest first.
func TestTheBackgroundListIsTheCountedShellsAndTheSubagentsOldestFirst(t *testing.T) {
	p, home := rigWith(t, func(*Config) {})
	p.Lifecycle(cfgOf(p), LifecycleConfig{Env: &supervisor.Env{DB: p.db, Home: home, Log: func(string, ...any) {}}})
	db.SetAgentState(p.db, "alice", states.Idle, 0, "")
	now := time.Now().Unix()
	shell := func(cmd string, at int64) {
		t.Helper()
		if _, err := p.db.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, 'alice', ?, json_object('command', ?))", at,
			session.BackgroundKind, cmd); err != nil {
			t.Fatal(err)
		}
	}
	sub := func(what string, at int64, bg bool) {
		t.Helper()
		if err := db.RecordSubagentStart(p.db, "alice", "", "s1", at, db.SubagentMeta{Description: what, Background: bg}); err != nil {
			t.Fatal(err)
		}
	}
	shell("old", now-50) // past the footer's count
	shell("one", now-40)
	sub("fore", now-30, false)
	shell("two", now-20)
	sub("back", now-10, true)
	list := func() string {
		var out []string
		for _, j := range p.backgroundJobs(home, "alice", now) {
			s := j.Kind + ":" + j.Label
			if j.Foreground != nil {
				s += fmt.Sprintf(":%v", *j.Foreground)
			}
			out = append(out, s)
		}
		return strings.Join(out, " ")
	}
	if got := list(); got != "subagent:fore:true subagent:back:false" {
		t.Fatalf("no shells while the footer reports no work: %q", got)
	}
	db.SetCursor(p.db, "bgwork:alice", 1)
	db.SetCursor(p.db, "bgcount:alice", 2)
	if got := list(); got != "shell:one subagent:fore:true shell:two subagent:back:false" {
		t.Fatalf("the footer's work: %q", got)
	}
}
