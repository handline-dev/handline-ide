package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/storetest"
)

// A founder's message typed into the agent's terminal and left unanswered holds its Stop once: never again for that message, again for
// a new one, not for one answered with a say or a Stop the hook already held, and an error lets the Stop through.
// founderStops is a rig for the Stop hook's founder nudge: the founder's message typed into a.one's terminal and landed at landed, a Stop
// recorded as the daemon records it, the nudge asked for a Stop input (more holds its fields after stop_hook_active), and its text.
type founderStops struct {
	t *testing.T
	d *sql.DB
	e *Env
}

func newFounderStops(t *testing.T) *founderStops {
	d := storetest.Open(t)
	return &founderStops{t: t, d: d, e: &Env{DB: d, Home: t.TempDir()}}
}

func (f *founderStops) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.d.Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *founderStops) typed(landed int64) int64 {
	f.t.Helper()
	rid, err := db.AddRequest(f.d, db.NewRequest{Requester: db.Human, Target: "a.one", Kind: "say", Body: "how is it going?"})
	if err != nil {
		f.t.Fatal(err)
	}
	f.exec("UPDATE requests SET founder_auth = 1, landed_at = ?, state = 'landed' WHERE id = ?", landed, rid)
	return rid
}

func (f *founderStops) stopped(at int64) {
	f.exec("INSERT INTO events(ts, agent, kind) VALUES(?, 'a.one', 'Stop')", at)
}

func (f *founderStops) stop(active bool, more string) string {
	f.t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"hook_event_name": "Stop", "session_id": "s1", "stop_hook_active": %v%s}`, active, more)), &raw); err != nil {
		f.t.Fatal(err)
	}
	return f.e.FounderNudge("a.one", raw)
}

func (f *founderStops) held(rid int64) string { return FounderNudgeText(db.MsgName(f.d, rid)) }

func TestAFoundersUnansweredMessageHoldsTheStopOncePerMessage(t *testing.T) {
	f := newFounderStops(t)
	d, exec, typed, stopped, held := f.d, f.exec, f.typed, f.stopped, f.held
	stop := func(active bool) string { t.Helper(); return f.stop(active, "") }

	first := typed(100)
	if out := stop(false); out != held(first) {
		t.Fatalf("an unanswered message holds the stop: %s", out)
	}
	stopped(101)
	if out := stop(false); out != "" {
		t.Fatalf("the same message is never nudged again: %s", out)
	}
	typed(90)
	if out := stop(false); out != "" {
		t.Fatalf("a message that landed before the last stop is not this turn's: %s", out)
	}
	second := typed(102)
	if out := stop(false); out != held(second) {
		t.Fatalf("a new message is nudged again: %s", out)
	}
	stopped(103)
	typed(104)
	answer, err := db.AddRequest(d, db.NewRequest{Requester: "a.one", Target: db.Human, Kind: "say", Body: "going well"})
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE requests SET ts = 105 WHERE id = ?", answer)
	if out := stop(false); out != "" {
		t.Fatalf("an answered message is not nudged: %s", out)
	}
	stopped(106)
	exec("UPDATE requests SET no_work = 'thanks' WHERE id = ?", typed(107))
	exec("INSERT INTO tasks(id, title, owner, opened_by, created_at, from_request) VALUES(8, 'asked', 'a.one', 'human', 1, ?)", typed(108))
	exec("INSERT INTO task_requests(task_id, request_id) VALUES(8, ?)", typed(109))
	exec("UPDATE requests SET founder_auth = 0 WHERE id = ?", typed(109))
	if out := stop(false); out != "" {
		t.Fatalf("a no-work mark, a task made from it or linked to it, or a message not typed by the founder is no nudge: %s", out)
	}
	third := typed(110)
	if out := stop(true); out != "" {
		t.Fatalf("a stop the hook already held passes: %s", out)
	}
	if out := stop(false); out != held(third) {
		t.Fatalf("the held stop left the message unanswered: %s", out)
	}
	typed(111)
	exec("DROP TABLE task_requests")
	if out := stop(false); out != "" {
		t.Fatalf("an error lets the stop through: %s", out)
	}
}

// A Stop that only pauses for the session's own background task or scheduled wakeup puts the nudge off once per message, though the
// daemon records that Stop: the next Stop nudges, even one listing a recurring cron again. Empty lists are a plain Stop.
func TestAStopPausedForItsOwnBackgroundWorkPutsTheNudgeOffOnce(t *testing.T) {
	f := newFounderStops(t)
	typed, stopped, held := f.typed, f.stopped, f.held
	stop := func(lists string) string { t.Helper(); return f.stop(false, lists) }
	shell := `, "background_tasks": [{"id": "b1", "type": "shell", "status": "running", "command": "go test ./..."}], "session_crons": []`
	loop := `, "background_tasks": [], "session_crons": [{"id": "c1", "schedule": "*/10 * * * *", "recurring": true, "prompt": "check"}]`

	first := typed(100)
	if out := stop(shell); out != "" {
		t.Fatalf("a stop paused on its own task is not nudged: %s", out)
	}
	stopped(101)
	if out := stop(`, "background_tasks": [], "session_crons": []`); out != held(first) {
		t.Fatalf("the stop after the wakeup is nudged: %s", out)
	}
	stopped(102)
	second := typed(103)
	if out := stop(loop); out != "" {
		t.Fatalf("a stop paused on its own wakeup is not nudged: %s", out)
	}
	stopped(104)
	if out := stop(loop); out != held(second) {
		t.Fatalf("a recurring cron puts the nudge off only once: %s", out)
	}
}
