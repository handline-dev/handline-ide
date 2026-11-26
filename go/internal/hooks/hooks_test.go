package hooks

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

func testRecorder(t *testing.T) *Recorder {
	t.Helper()
	home := t.TempDir()
	db, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Recorder{DB: db, Home: home, Log: func(string, ...any) {}}
}

func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAHookAppliesOnceByRequestIDAndALateOneMovesNothing(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	apply := func(kind, rid string, at int64) {
		t.Helper()
		if err := Apply(h.DB, h.Home, "w", kind, map[string]any{}, rid, at); err != nil {
			t.Fatal(err)
		}
	}
	apply("UserPromptSubmit", "a", now-10)
	apply("UserPromptSubmit", "a", now-9) // the same id
	apply("Stop", "b", now-5)             // RUNNING -> IDLE
	apply("UserPromptSubmit", "c", now-8) // late: recorded, nothing moves
	if n := count(t, h.DB, "SELECT COUNT(*) FROM events WHERE agent = 'w'"); n != 3 {
		t.Errorf("%d events, want 3", n)
	}
	var state string
	h.DB.QueryRow("SELECT state FROM agent_state WHERE agent = 'w'").Scan(&state)
	if state != "IDLE" {
		t.Errorf("state %s: a late event moved it", state)
	}
}

func TestOnlyAFreshSessionStartNamesTheLaunchItCameFrom(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	start := func(source, sid string) {
		t.Helper()
		if err := Apply(h.DB, h.Home, "w", "SessionStart", map[string]any{"source": source, "session_id": sid}, sid, now); err != nil {
			t.Fatal(err)
		}
	}
	named := func() string {
		var sid sql.NullString
		h.DB.QueryRow("SELECT session_id FROM session_starts WHERE agent = 'w' ORDER BY id LIMIT 1").Scan(&sid)
		return sid.String
	}
	if _, err := h.DB.Exec("INSERT INTO session_starts (agent, ts, cause, resumed, text, prompt) VALUES ('w', ?, 'wake', 0, '', '')", now); err != nil {
		t.Fatal(err)
	}
	start("clear", "s-clear")
	if got := named(); got != "" {
		t.Errorf("a /clear claimed the launch: %q", got)
	}
	start("startup", "s-new")
	if got := named(); got != "s-new" {
		t.Errorf("the new session did not name its launch: %q", got)
	}
	if _, err := h.DB.Exec("INSERT INTO session_starts (agent, ts, cause, resumed, text, prompt) VALUES ('w', ?, 'launch', 0, '', '')", now-7200); err != nil {
		t.Fatal(err)
	}
	start("startup", "s-late")
	if n := count(t, h.DB, "SELECT COUNT(*) FROM session_starts WHERE session_id = 's-late'"); n != 0 {
		t.Errorf("a launch too old was claimed")
	}
}

func TestTheSpoolSetsABadLineAsideAndAppliesTheRest(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	good := `{"agent": "w", "kind": "Stop", "at": ` + itoa(now-5) + `, "data": {}, "request_id": "g1"}`
	os.WriteFile(spoolPath(h.Home), []byte("garbage\n"+good+"\n"+good+"\n"), 0o600)
	n, err := h.Ingest()
	if err != nil || n != 2 {
		t.Fatalf("applied %d, %v", n, err)
	}
	if c := count(t, h.DB, "SELECT COUNT(*) FROM events WHERE agent = 'w'"); c != 1 {
		t.Errorf("%d events, want 1: a duplicate id applies once", c)
	}
	rej, _ := os.ReadFile(filepath.Join(h.Home, rejectedName))
	if !strings.Contains(string(rej), "garbage") {
		t.Errorf("rejected: %s", rej)
	}
	if _, err := os.Stat(ingesting(h.Home)); err == nil {
		t.Error("the ingest file was not removed")
	}
}

func TestAnIngestThatFailsKeepsTheLinesItDidNotApply(t *testing.T) {
	h := testRecorder(t)
	line := func(id string) string {
		return `{"agent": "w", "kind": "Stop", "at": 1, "data": {}, "request_id": "` + id + `"}`
	}
	os.WriteFile(ingesting(h.Home), []byte(line("k1")+"\n"+line("k2")+"\n"), 0o600)
	h.DB.Close() // every apply now fails: not the lines' fault
	if _, err := h.Ingest(); err == nil {
		t.Fatal("no error from a closed database")
	}
	b, _ := os.ReadFile(ingesting(h.Home))
	if !strings.Contains(string(b), "k1") || !strings.Contains(string(b), "k2") {
		t.Errorf("lines were lost: %s", b)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestAnUnansweredPermissionPromptIsNotedUntilTheAgentMovesOn(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	rid := 0
	apply := func(kind string, raw map[string]any) {
		t.Helper()
		rid++
		if err := Apply(h.DB, h.Home, "w", kind, raw, strconv.Itoa(rid), now); err != nil {
			t.Fatal(err)
		}
	}
	prompt := map[string]any{"notification_type": "permission_prompt", "message": "Claude needs your permission"}
	apply("Notification", map[string]any{"notification_type": "idle_prompt"})
	if _, ok := Notified(h.Home, "w"); ok {
		t.Fatal("an idle notification is no prompt")
	}
	for _, next := range []string{"PostToolUse", "PostToolUseFailure", "Stop", "StopFailure", "UserPromptSubmit", "SessionEnd"} {
		apply("Notification", prompt)
		if since, ok := Notified(h.Home, "w"); !ok || since != now {
			t.Fatalf("a permission prompt left unanswered is noted: %d %v", since, ok)
		}
		apply(next, map[string]any{"tool_name": "Bash"})
		if _, ok := Notified(h.Home, "w"); ok {
			t.Errorf("%s: the agent moved on, and the prompt is still noted", next)
		}
	}
	if n := count(t, h.DB, "SELECT COUNT(*) FROM events WHERE agent = 'w' AND kind = 'Notification' AND payload LIKE '%permission_prompt%'"); n != 6 {
		t.Errorf("%d permission_prompt events recorded, want 6", n)
	}
}

func TestTheViewIsToldWhenAHookRaisesOrClearsTheNeedsYouCard(t *testing.T) {
	h := testRecorder(t)
	h.Turns = &turns.Tracker{Agent: func(string) string { return "w" }}
	var told int
	h.Turns.OnChange(func(string) { told++ })
	now := time.Now().Unix()
	apply := func(kind, rid string, raw map[string]any, want int) {
		t.Helper()
		told = 0
		if err := h.apply("w", kind, raw, rid, now); err != nil {
			t.Fatal(err)
		}
		if told != want {
			t.Errorf("%s: the view was told %d times, want %d", kind, told, want)
		}
	}
	apply("UserPromptSubmit", "a", map[string]any{}, 1)
	apply("Notification", "b", map[string]any{"notification_type": "permission_prompt"}, 1)
	apply("PostToolUse", "c", map[string]any{"tool_name": "Bash"}, 1)
	apply("PostToolUse", "d", map[string]any{"tool_name": "Bash"}, 0)
}

func TestAnInterruptIsWhatAStopWouldHaveDone(t *testing.T) {
	h := testRecorder(t)
	h.Turns = &turns.Tracker{Agent: func(string) string { return "w" }}
	now := time.Now().Unix()
	if err := h.apply("w", "UserPromptSubmit", map[string]any{}, "a", now); err != nil {
		t.Fatal(err)
	}
	if idle, known := h.Turns.WaitIdle("%1", 0); idle || !known {
		t.Fatalf("the tracker was not told the prompt: idle=%v known=%v", idle, known)
	}
	noteWait(h.Home, "w", "Bash", map[string]any{"command": "ls"}, 1)
	if err := MarkInterrupted(h.DB, h.Home, "w"); err != nil {
		t.Fatal(err)
	}
	var state, detail string
	h.DB.QueryRow("SELECT state, detail FROM agent_state WHERE agent = 'w'").Scan(&state, &detail)
	if state != "IDLE" || detail != "interrupted its turn" || ReadWait(h.Home, "w") != nil {
		t.Fatalf("%s %q, wait %v", state, detail, ReadWait(h.Home, "w"))
	}
	h.DB.Exec("UPDATE agent_state SET state = 'STALLED' WHERE agent = 'w'")
	if err := MarkInterrupted(h.DB, h.Home, "w"); err != nil {
		t.Fatal(err)
	}
	h.DB.QueryRow("SELECT state FROM agent_state WHERE agent = 'w'").Scan(&state)
	if state != "STALLED" {
		t.Fatalf("an interrupt moved a state the daemon owns: %s", state)
	}
}

// The call that ran (or failed) after its permission question was answered at the agent's own dialog closes that question; a parallel
// call to another tool or command leaves it open.
func TestAToolCallThatRanClosesItsPermissionQuestion(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	open := func() int {
		return count(t, h.DB, "SELECT COUNT(*) FROM prompt_blocks WHERE agent = 'w' AND gone_at IS NULL")
	}
	for i, c := range []struct {
		event, tool, command string
		closes               bool
	}{
		{"PostToolUse", "Bash", "other", false},
		{"PostToolUse", "Read", "cd /x && make", false},
		{"PostToolUse", "Bash", "cd /x && make", true},
		{"PostToolUseFailure", "Bash", "cd /x && make", true},
	} {
		if _, err := h.DB.Exec("INSERT INTO prompt_blocks(agent, kind, fingerprint, title, panel, detail, answerable, first_seen, last_seen) "+
			"VALUES('w','permission',?, 't','[]', ?, 1, ?, ?)", "fp"+strconv.Itoa(i),
			`{"source":"hook","tool":"Bash","command":"cd /x && make"}`, now, now); err != nil {
			t.Fatal(err)
		}
		in := map[string]any{"command": c.command}
		if err := Apply(h.DB, h.Home, "w", c.event, map[string]any{"tool_name": c.tool, "tool_input": in}, "r"+strconv.Itoa(i), now+int64(i)); err != nil {
			t.Fatal(err)
		}
		if got := open() == 0; got != c.closes {
			t.Fatalf("%+v: closed %v", c, got)
		}
		h.DB.Exec("DELETE FROM prompt_blocks")
	}
}

func TestAStopAsksForTheNudgeBeforeItIsRecordedAndAPostToolUseTellsBackground(t *testing.T) {
	h := testRecorder(t)
	var order []string
	h.Nudge = func(agent string, raw map[string]any) string {
		order = append(order, "nudge")
		if n := count(t, h.DB, "SELECT COUNT(*) FROM events WHERE agent = 'w' AND kind = 'Stop'"); n != 0 {
			t.Errorf("the Stop was recorded before the nudge was asked: %d", n)
		}
		return "answer it"
	}
	h.Background = func(agent string, raw map[string]any) { order = append(order, "background:"+agent) }
	call := func(kind, rid string) map[string]any {
		t.Helper()
		got, err := h.record(&rpc.Conn{}, map[string]any{"agent": "w", "kind": kind, "at": json.Number(strconv.FormatInt(time.Now().Unix(), 10)),
			"data": map[string]any{}, "request_id": rid})
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)
	}
	if got := call("Stop", "s1"); got["nudge"] != "answer it" {
		t.Fatalf("result %v", got)
	}
	if got := call("PostToolUse", "p1"); got["nudge"] != nil {
		t.Fatalf("a PostToolUse is nudged: %v", got)
	}
	if want := "nudge background:w"; strings.Join(order, " ") != want {
		t.Fatalf("order %v, want %s", order, want)
	}
}

func TestATurnsEndIsToldOnceApplied(t *testing.T) {
	h := testRecorder(t)
	var told []string
	h.TurnEnded = func(agent, kind string) { told = append(told, agent+":"+kind) }
	for i, kind := range []string{"PostToolUse", "Stop", "StopFailure", "SubagentStop", "SessionEnd"} {
		if err := h.apply("w", kind, map[string]any{}, "t"+strconv.Itoa(i), time.Now().Unix()); err != nil {
			t.Fatal(err)
		}
	}
	if want := "w:Stop w:StopFailure w:SessionEnd"; strings.Join(told, " ") != want {
		t.Fatalf("told %v, want %s", told, want)
	}
}

// A tool use moves a queued row to RUNNING unless the move machinery queued it: its park reason says so, not its detail's words.
func TestAToolUseLeavesAMovingRowQueued(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	for agent, reason := range map[string]any{"mover": "moving", "worded": nil} {
		if _, err := h.DB.Exec("INSERT INTO agent_state(agent, state, detail, updated, park_reason) VALUES(?, 'QUEUED', 'moving a -> b', 0, ?)", agent, reason); err != nil {
			t.Fatal(err)
		}
		if err := h.apply(agent, "PostToolUse", map[string]any{"tool_name": "Bash"}, "r-"+agent, now); err != nil {
			t.Fatal(err)
		}
	}
	for agent, want := range map[string]string{"mover": "QUEUED", "worded": "RUNNING"} {
		var got string
		if err := h.DB.QueryRow("SELECT state FROM agent_state WHERE agent = ?", agent).Scan(&got); err != nil || got != want {
			t.Errorf("%s: %q %v, want %s", agent, got, err, want)
		}
	}
}

func TestAStopKeepsWhatItSaidStillRunsCutShort(t *testing.T) {
	h := testRecorder(t)
	long := strings.Repeat("x", 300)
	tasks := []any{map[string]any{"id": "b1", "type": "shell", "status": "running", "description": "", "command": "curl -H 'Authorization: x'", "output_file": "/tmp/o"},
		"not an entry", map[string]any{"id": "a1", "type": "local_agent", "status": "running", "description": long}}
	for i := 0; i < 30; i++ {
		tasks = append(tasks, map[string]any{"id": "c" + strconv.Itoa(i), "type": "shell", "status": "completed"})
	}
	if err := Apply(h.DB, h.Home, "w", "Stop", map[string]any{"background_tasks": tasks}, "a", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	var raw string
	h.DB.QueryRow("SELECT json_extract(payload, '$.background_tasks') FROM events WHERE agent = 'w' AND kind = 'Stop'").Scan(&raw)
	var got []map[string]string
	if err := json.Unmarshal([]byte(raw), &got); err != nil || len(got) != 20 || strings.Contains(raw, "Authorization") {
		t.Fatalf("%d kept (%v): %s", len(got), err, raw)
	}
	if b := got[0]; len(b) != 3 || b["id"] != "b1" || b["status"] != "running" { // no empty description, no command, no output_file
		t.Fatalf("the shell %v", b)
	}
	if a := got[1]; a["id"] != "a1" || len([]rune(a["description"])) != 100 { // the string entry is skipped
		t.Fatalf("the subagent %v", a)
	}
}

func TestAStopListingRunningWorkSetsTheBackgroundCursorsAtOnce(t *testing.T) {
	h := testRecorder(t)
	cursor := func(name string) (int64, int64) {
		var id, at int64
		h.DB.QueryRow("SELECT row_id, updated FROM cursors WHERE name = ?", name).Scan(&id, &at)
		return id, at
	}
	running := map[string]any{"id": "b1", "type": "shell", "status": "running"}
	hook := func(event, rid string, at int64, tasks ...any) {
		t.Helper()
		if err := Apply(h.DB, h.Home, "w", event, map[string]any{"background_tasks": tasks}, rid, at); err != nil {
			t.Fatal(err)
		}
	}
	stop := func(rid string, at int64, tasks ...any) { t.Helper(); hook("Stop", rid, at, tasks...) }
	now := time.Now().Unix()
	hook("SubagentStop", "s", now-40, running) // it carries the session's list too, mid-turn
	if id, _ := cursor("bgwork:w"); id != 0 {
		t.Fatal("a SubagentStop set the cursor")
	}
	stop("a", now-30, map[string]any{"id": "c1", "type": "shell", "status": "completed"})
	if id, _ := cursor("bgwork:w"); id != 0 {
		t.Fatal("a Stop with nothing running set the cursor")
	}
	stop("b", now-20, running, map[string]any{"id": "c1", "type": "shell", "status": "completed"})
	if id, at := cursor("bgwork:w"); id != 1 || at != now-20 {
		t.Fatalf("bgwork %d at %d", id, at)
	}
	if n, _ := cursor("bgcount:w"); n != 1 {
		t.Fatalf("bgcount %d", n)
	}
	stop("c", now-10, running, map[string]any{"id": "b2", "type": "shell", "status": "running"})
	if _, at := cursor("bgwork:w"); at != now-20 {
		t.Fatalf("the cursor lost when the work was first seen: %d", at)
	}
	if n, _ := cursor("bgcount:w"); n != 2 {
		t.Fatalf("bgcount %d", n)
	}
}

func TestASubagentStartPassesOnWhatTheAgentCallSaid(t *testing.T) {
	h := testRecorder(t)
	var got SubagentCall
	h.Subagent = func(_, _ string, m SubagentCall) (string, error) { got = m; return "", nil }
	if _, err := h.subagentStart(&rpc.Conn{}, map[string]any{"agent": "w", "session_id": "s", "description": "review the diff",
		"subagent_type": "general-purpose", "run_in_background": true}); err != nil {
		t.Fatal(err)
	}
	if got != (SubagentCall{Description: "review the diff", Type: "general-purpose", Background: true}) {
		t.Fatalf("%+v", got)
	}
}

// A tool call is no event, but the agent is heard: LastActivity moves with it, and only forward.
func TestAToolCallIsHeardWithoutAnEvent(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	for i, at := range []int64{now - 600, now, now - 60} {
		if err := h.apply("w", []string{"PostToolUse", "PostToolUseFailure", "PostToolUse"}[i], map[string]any{"tool_name": "Bash"}, "t"+strconv.Itoa(i), at); err != nil {
			t.Fatal(err)
		}
	}
	if last, ok, err := db.LastActivity(h.DB, "w"); err != nil || !ok || last != now {
		t.Fatalf("last heard %d %v %v, want %d", last, ok, err, now)
	}
	if n := count(t, h.DB, "SELECT COUNT(*) FROM events WHERE agent = 'w'"); n != 0 {
		t.Fatalf("%d events from tool calls", n)
	}
}

func TestAHeavyBashCallIsKeptByItsShapeAndAWeekOldOneGoes(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	post := func(rid string, heavy map[string]any, at int64) {
		t.Helper()
		raw := map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "tools/check.sh"}}
		if heavy != nil {
			raw[Heavy] = heavy
		}
		if err := Apply(h.DB, h.Home, "w", "PostToolUse", raw, rid, at); err != nil {
			t.Fatal(err)
		}
	}
	post("old", map[string]any{"shape": "check.sh", "seconds": json.Number("600")}, now-8*24*3600)
	post("light", nil, now-20)
	post("heavy", map[string]any{"shape": "check.sh", "seconds": json.Number("1200")}, now-10)
	if n := count(t, h.DB, "SELECT COUNT(*) FROM heavy_runs WHERE agent = 'w' AND shape = 'check.sh' AND seconds = 1200 AND ended = ?", now-10); n != 1 {
		t.Errorf("%d heavy runs kept", n)
	}
	if n := count(t, h.DB, "SELECT COUNT(*) FROM heavy_runs"); n != 1 {
		t.Errorf("%d rows: the week-old one stayed or the light one was kept", n)
	}
}

// Every timed Bash call is counted under its shape's day, long or short, so a shape's share of long runs is known; a heavy run that
// failed at its own time limit is kept as timed out.
func TestEveryTimedCallIsCountedUnderItsShapeAndATimeoutIsKept(t *testing.T) {
	h := testRecorder(t)
	now := time.Now().Unix()
	day := now / (24 * 3600)
	post := func(rid, event string, extra map[string]any, at int64) {
		t.Helper()
		raw := map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "tools/check.sh"}}
		for k, v := range extra {
			raw[k] = v
		}
		if err := Apply(h.DB, h.Home, "w", event, raw, rid, at); err != nil {
			t.Fatal(err)
		}
	}
	post("short", "PostToolUse", map[string]any{Ran: map[string]any{"shape": "check.sh", "long": false}}, now-30)
	post("long", "PostToolUse", map[string]any{Ran: map[string]any{"shape": "check.sh", "long": true},
		Heavy: map[string]any{"shape": "check.sh", "seconds": json.Number("300")}}, now-20)
	post("out", "PostToolUseFailure", map[string]any{Ran: map[string]any{"shape": "check.sh", "long": true},
		Heavy: map[string]any{"shape": "check.sh", "seconds": json.Number("600"), "timed_out": true}}, now-10)
	post("untimed", "PostToolUse", nil, now-5)
	post("old", "PostToolUse", map[string]any{Ran: map[string]any{"shape": "old.sh", "long": true}}, now-9*24*3600)
	post("newer", "PostToolUse", map[string]any{Ran: map[string]any{"shape": "git status", "long": false}}, now-1)
	var runs, long int
	if err := h.DB.QueryRow("SELECT SUM(runs), SUM(long) FROM shape_days WHERE shape = 'check.sh' AND day >= ?", day-1).Scan(&runs, &long); err != nil {
		t.Fatal(err)
	}
	if runs != 3 || long != 2 {
		t.Errorf("check.sh: %d runs, %d long", runs, long)
	}
	if n := count(t, h.DB, "SELECT COUNT(*) FROM shape_days WHERE shape = 'old.sh'"); n != 0 {
		t.Errorf("a day older than a week stayed")
	}
	if n := count(t, h.DB, "SELECT COUNT(*) FROM heavy_runs WHERE timed_out = 1 AND seconds = 600"); n != 1 {
		t.Errorf("%d runs kept as timed out", n)
	}
	if n := count(t, h.DB, "SELECT COUNT(*) FROM heavy_runs WHERE timed_out = 0 AND seconds = 300"); n != 1 {
		t.Errorf("%d runs kept as ended in time", n)
	}
}

// A turn that ended on an error leaves the agent idle, so the wake for an idle agent with work sends it on; a usage limit says so.
func TestATurnEndedOnAnErrorLeavesTheAgentIdleAndSettled(t *testing.T) {
	h := testRecorder(t)
	h.Turns = &turns.Tracker{Agent: func(string) string { return "w" }}
	at := time.Now().Unix() - 10
	for i, c := range []struct{ err, detail string }{
		{"server_error", "its turn ended on an error: server_error"},
		{"rate_limit", "at its usage limit: rate_limit"},
	} {
		if err := h.apply("w", "UserPromptSubmit", map[string]any{}, "p"+strconv.Itoa(i), at); err != nil {
			t.Fatal(err)
		}
		if err := h.apply("w", "StopFailure", map[string]any{"error": c.err}, "f"+strconv.Itoa(i), at+1); err != nil {
			t.Fatal(err)
		}
		at += 2
		var state, detail string
		h.DB.QueryRow("SELECT state, detail FROM agent_state WHERE agent = 'w'").Scan(&state, &detail)
		if state != "IDLE" || detail != c.detail {
			t.Fatalf("%s: %s %q", c.err, state, detail)
		}
		// Only a usage limit may leave a dialog up: the screen is still read for one.
		if settled := h.Turns.Settled("%1"); settled != (c.err != "rate_limit") {
			t.Fatalf("%s: settled %v", c.err, settled)
		}
	}
}
