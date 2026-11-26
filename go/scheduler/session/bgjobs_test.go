package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/kit/storetest"
)

func TestFooterCountAddsEveryNumber(t *testing.T) {
	for in, want := range map[string]int{"2 shells, 1 monitor": 3, "": 0, "no jobs": 0, "10 shells": 10} {
		if got := FooterCount(in); got != want {
			t.Errorf("FooterCount(%q) = %d, want %d", in, got, want)
		}
	}
}

func bgCall(command string, response any) map[string]any {
	return map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": command, "run_in_background": true},
		"tool_response": response, "tool_use_id": "tu1", "session_id": "s1"}
}

func TestOnlyABackgroundBashCallIsNoted(t *testing.T) {
	f := newFixture(t)
	for _, raw := range []map[string]any{
		{"tool_name": "Edit", "tool_input": map[string]any{"run_in_background": true}},
		{"tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}},
		{"tool_name": "Bash", "tool_input": map[string]any{"command": "ls", "run_in_background": "yes"}},
		{"tool_name": "Bash"},
	} {
		if err := f.NoteBackground("worker", raw); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	f.DB.QueryRow("SELECT COUNT(*) FROM events").Scan(&n)
	if n != 0 {
		t.Fatalf("%d events for calls that started nothing", n)
	}
}

func TestRunningListsTheJobsWithNoExitLine(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	out := func(id, body string) string {
		p := filepath.Join(dir, "sess", "tasks", id+".output")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
		return p
	}
	done := out("aaa", strings.Repeat("x", 500)+"\n[exited with code 0]\n")
	going := out("bbb", "still\n")
	f.NoteBackground("worker", bgCall("sleep 1", fmt.Sprintf("started, output in %s", done)))
	f.now++
	f.NoteBackground("worker", bgCall("make   watch", fmt.Sprintf(`{"backgroundTaskId": "bbb", "out": "%s"}`, going)))
	f.now++
	tail := bgCall("tail -f log", map[string]any{"shellId": "ccc"})
	tail["tool_input"].(map[string]any)["description"] = "Follow the log"
	f.NoteBackground("worker", tail)
	f.now++
	f.NoteBackground("worker", bgCall("nothing said", "ok"))

	jobs, err := f.RunningJobs("worker", 0)
	if err != nil {
		t.Fatal(err)
	}
	var cmds, ids []string
	for _, j := range jobs {
		cmds, ids = append(cmds, j.Command), append(ids, j.Job)
	}
	if got := strings.Join(cmds, "|"); got != "make   watch|tail -f log|nothing said" {
		t.Errorf("running: %q", got)
	}
	if got := strings.Join(ids, "|"); got != "bbb|ccc|" {
		t.Errorf("job ids: %q", got)
	}
	if last, _ := f.RunningJobs("worker", 2); len(last) != 2 || last[0].Command != "tail -f log" || last[0].Description != "Follow the log" ||
		last[1].Description != "" {
		t.Errorf("last two: %v", last)
	}

	// The footer showing none ends everything recorded before it.
	f.now++
	f.event("worker", "background_work_ended")
	if after, _ := f.RunningJobs("worker", 0); len(after) != 0 {
		t.Errorf("jobs survived the footer showing none: %v", after)
	}
}

func TestAJobsLabelIsItsDescriptionElseItsCommand(t *testing.T) {
	if l := (BgJob{Command: "tools/check.sh > x", Description: "Run the full check"}).Label(); l != "Run the full check" {
		t.Fatal(l)
	}
	if l := (BgJob{Command: "sleep  60", Description: " "}).Label(); l != "sleep 60" {
		t.Fatal(l)
	}
	if l := (BgJob{Description: strings.Repeat("é", 150)}).Label(); utf8.RuneCountInString(l) != CommandWidth {
		t.Fatal(utf8.RuneCountInString(l))
	}
}

// A background shell still runs while its output is there and has not ended; one that ended, one whose output is gone and one of
// no shape are not told.
func TestOpenBackgroundsAreTheShellsWhoseOutputHasNotEnded(t *testing.T) {
	q, now := storetest.Open(t), time.Now().Unix()
	dir := t.TempDir()
	write := func(name, text string) string {
		f := filepath.Join(dir, name)
		if err := os.WriteFile(f, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	running, ended := write("running.output", "still going\n"), write("ended.output", "ok\n\n[exited with code 0]\n")
	for _, p := range []string{
		`{"command": "tools/check.sh origin/main", "job": "bjob1", "session": "s", "output": "` + running + `"}`,
		`{"command": "tools/check.sh", "job": "bjob2", "session": "s", "output": "` + ended + `"}`,
		`{"command": "tools/check.sh", "job": "bjob3", "session": "s", "output": "` + filepath.Join(dir, "gone.output") + `"}`,
		`{"command": "sleep 600", "job": "bjob4", "session": "s", "output": "` + running + `"}`,
	} {
		if _, err := q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, 'a8', 'background_started', ?)", now-100, p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := OpenBackgrounds(q, now-1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (OpenBackground{Agent: "a8", Job: "bjob1", Started: now - 100, Shape: "check.sh"}) {
		t.Errorf("%+v", got)
	}
	if got, _ := OpenBackgrounds(q, now-10); len(got) != 0 {
		t.Errorf("started before since: %+v", got)
	}
}
