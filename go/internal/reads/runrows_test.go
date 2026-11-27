package reads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/internal/cmdruns"
)

func TestATasksTimelineCarriesTheRunsAskedWhileDoingIt(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'bob', 'amy', 10)")
	run(t, e, "INSERT INTO task_comments(task_id, author, body, created_at) VALUES(1, 'amy', 'a comment', 25)")
	dir := filepath.Join(e.Home, "proposals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct{ id, kind, task, created, state string }{
		{"0000000a", "run", "1", "20", "rejected"},
		{"0000000b", "run", "1", "30", "applied"},
		{"0000000c", "run", "2", "15", "proposed"},    // another task's
		{"0000000d", "run", "", "16", "proposed"},     // asked while doing none
		{"0000000e", "run", "0", "17", "proposed"},    // no task
		{"0000000f", "retire", "1", "18", "proposed"}, // not a command
	} {
		task := ""
		if p.task != "" {
			task = "task: " + p.task + "\n"
		}
		body := fmt.Sprintf("id: %s\nkind: %s\nagent: bob\nrequested_by: bob\nwhy: w\ncreated: %s\nstate: %s\n%sset.command: '\"echo %s\"'\nset.cwd: /tmp\n",
			p.id, p.kind, p.created, p.state, task, p.id)
		if err := os.WriteFile(filepath.Join(dir, p.id+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r, err := cmdruns.Start(e.DB, cmdruns.ProposalBlock(0xb), "%9", "bob", "echo 0000000b", 31)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmdruns.Replaced(e.DB, r, term.PaneRun{Dead: true, Status: 3, DeadTime: 32}, 32, nil); err != nil {
		t.Fatal(err)
	}
	got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, x := range got.(map[string]any)["timeline"].([]any) {
		m := x.(map[string]any)
		if m["kind"] != "run" {
			rows = append(rows, fmt.Sprint(m["kind"]))
			continue
		}
		p := m["proposal"].(map[string]any)
		run, _ := p["run"].(map[string]any)
		rows = append(rows, fmt.Sprintf("run|%v|%v|%v|%v|%v|%v", m["who"], m["text"], p["task"], p["state"], run["state"], run["exit"]))
	}
	want := []string{"run|bob|echo 0000000a|1|rejected|<nil>|<nil>", "memo", "run|bob|echo 0000000b|1|approved|failed|3"}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Fatalf("got  %q\nwant %q", rows, want)
	}
}

func TestARunWrittenAfterATaskWasReadShowsOnItsNextRead(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'bob', 'amy', 10)")
	dir := filepath.Join(e.Home, "proposals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(id, state string) { // as the store writes: a new file renamed in, at one fixed time
		body := fmt.Sprintf("id: %s\nkind: run\nagent: bob\nrequested_by: bob\nwhy: w\ncreated: 20\nstate: %s\ntask: 1\nset.command: '\"echo %s\"'\nset.cwd: /tmp\n", id, state, id)
		tmp := filepath.Join(dir, id+".tmp")
		if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Unix(100, 0)
		if err := os.Chtimes(tmp, at, at); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, filepath.Join(dir, id+".yaml")); err != nil {
			t.Fatal(err)
		}
	}
	states := func() string {
		got, err := e.taskGet(nil, map[string]any{"task": json.Number("1")})
		if err != nil {
			t.Fatal(err)
		}
		var s []string
		for _, x := range got.(map[string]any)["timeline"].([]any) {
			if m := x.(map[string]any); m["kind"] == "run" {
				s = append(s, fmt.Sprint(m["text"], "|", m["proposal"].(map[string]any)["state"]))
			}
		}
		return fmt.Sprint(s)
	}
	write("0000000a", "proposed")
	if got := states(); got != "[echo 0000000a|open]" {
		t.Fatalf("first read: %s", got)
	}
	write("0000000a", "rejected")
	write("0000000b", "proposed")
	if got := states(); got != "[echo 0000000a|rejected echo 0000000b|open]" {
		t.Fatalf("after the writes: %s", got)
	}
}
