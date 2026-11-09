package watcher

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/handline/handline/go/agentctl/agent/vendors"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/cmdruns"
	"github.com/handline/handline/go/internal/reads"
)

type chatRig struct {
	t    *testing.T
	d    *sql.DB
	l    *stream.Log
	emit Emit
}

func newChatRig(t *testing.T) *chatRig {
	d, l := rig(t)
	return &chatRig{t, d, l, func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }}
}

func (r *chatRig) chat() *Chat { return &Chat{Reads: &reads.Env{DB: r.d}} }

func (r *chatRig) scan(c *Chat) {
	r.t.Helper()
	if err := c.Scan(r.d, r.emit); err != nil {
		r.t.Fatal(err)
	}
}

func (r *chatRig) rows() []*stream.Event {
	r.t.Helper()
	got, err := r.l.Since(0, []string{"chat"}, "", 100)
	if err != nil {
		r.t.Fatal(err)
	}
	return got
}

func (r *chatRig) types() []string {
	var out []string
	for _, e := range r.rows() {
		out = append(out, e.Type+":"+*e.Agent)
	}
	return out
}

func wantTypes(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestAChangedRequestIsOneChatMessageToEachAgentOfItAndNotToTheFounder(t *testing.T) {
	r := newChatRig(t)
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 100, 'human', 'bob', 'old', 'queued', 'say')")
	c := r.chat()
	r.scan(c)
	wantTypes(t, r.types()) // the first look says nothing, not even for the newest second
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(2, 200, 'amy', 'bob', 'hello', 'queued', 'say')")
	r.scan(c)
	r.scan(c)
	wantTypes(t, r.types(), "chat.message:bob", "chat.message:amy")
	got := r.rows()[0]
	var m map[string]any
	if json.Unmarshal(got.Data, &m) != nil || m["id"] != float64(2) || m["from"] != "amy" || m["body"] != "hello" || string(got.Target) != `{"id":"bob","screen":"agent"}` || got.Title != nil {
		t.Fatalf("%s", got.Data)
	}
	exec(t, r.d, "UPDATE requests SET state = 'landed', state_at = 300 WHERE id = 2")
	r.scan(c)
	wantTypes(t, r.types(), "chat.message:bob", "chat.message:amy", "chat.message:bob", "chat.message:amy")
}

func TestAReplyToTheFoundersMessageIsAMessageOfItsOwn(t *testing.T) {
	r := newChatRig(t)
	c := r.chat()
	r.scan(c)
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 100, 'human', 'bob', 'q', 'landed', 'say')")
	r.scan(c)
	wantTypes(t, r.types(), "chat.message:bob")
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind, parent_id) VALUES(2, 150, 'bob', 'human', 'the answer', 'landed', 'say', 1)")
	r.scan(c)
	wantTypes(t, r.types(), "chat.message:bob", "chat.message:bob")
	var m map[string]any
	if json.Unmarshal(r.rows()[1].Data, &m) != nil || m["id"] != float64(2) || m["parent"] != float64(1) {
		t.Fatalf("%s", r.rows()[1].Data)
	}
}

func TestARestartSaysNothingAgainButAChangeWhileDownIsSaid(t *testing.T) {
	r := newChatRig(t)
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 100, 'human', 'bob', 'a', 'queued', 'say')")
	c := r.chat()
	r.scan(c)
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(2, 200, 'human', 'bob', 'b', 'queued', 'say')")
	r.scan(c)
	wantTypes(t, r.types(), "chat.message:bob")
	again := r.chat() // the daemon restarted: its memory is gone, the cursor is not
	r.scan(again)
	r.scan(again)
	wantTypes(t, r.types(), "chat.message:bob")
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(3, 300, 'human', 'bob', 'c', 'queued', 'say')")
	r.scan(again)
	wantTypes(t, r.types(), "chat.message:bob", "chat.message:bob")
}

func TestATaskEventOfAnOwnedTaskIsOneTranscriptItemAndTheFoundersIsNone(t *testing.T) {
	r := newChatRig(t)
	exec(t, r.d, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', 'bob', 'human', 1), (2, 'u', 'human', 'bob', 1)")
	c := r.chat()
	r.scan(c)
	for _, id := range []int{1, 2} {
		exec(t, r.d, "INSERT INTO events(ts, agent, kind, payload) VALUES(5, 'bob', 'task_new', ?)", `{"task_id":`+string(rune('0'+id))+`}`)
	}
	exec(t, r.d, "INSERT INTO events(ts, agent, kind, payload) VALUES(6, 'bob', 'other_kind', '{\"task_id\":1}')")
	r.scan(c)
	r.scan(c)
	wantTypes(t, r.types(), "chat.transcript:bob")
	var item map[string]any
	json.Unmarshal(r.rows()[0].Data, &item)
	if item["kind"] != "task_event" || item["task_event"].(map[string]any)["event"] != "opened" {
		t.Fatalf("%v", item)
	}
}

func TestAPromptIsToldWhenItOpensAndOnceMoreWhenItCloses(t *testing.T) {
	r := newChatRig(t)
	ins := "INSERT INTO prompt_blocks(agent, kind, fingerprint, title, panel, detail, answerable, first_seen, last_seen) VALUES(?, 'question', 'f', 't', '[]', '{}', 1, 1, 1)"
	exec(t, r.d, ins, "amy") // open before the daemon: told closed when it closes
	c := r.chat()
	r.scan(c)
	exec(t, r.d, ins, "bob")
	r.scan(c)
	wantTypes(t, r.types(), "chat.transcript:bob")
	exec(t, r.d, "UPDATE prompt_blocks SET gone_at = 9 WHERE id = 2")
	exec(t, r.d, "UPDATE prompt_blocks SET gone_at = 9 WHERE id = 1")
	r.scan(c)
	r.scan(c)
	wantTypes(t, r.types(), "chat.transcript:bob", "chat.transcript:amy", "chat.transcript:bob")
	var open []any
	for _, e := range r.rows() {
		var it map[string]any
		json.Unmarshal(e.Data, &it)
		open = append(open, it["open"])
	}
	if open[0] != true || open[1] != false || open[2] != false {
		t.Fatalf("%v", open)
	}
}

// A closed prompt's item says its answer: what prompt.answer kept on the block, or, answered in the terminal, outside with when it closed.
func TestAClosedPromptCarriesItsAnswer(t *testing.T) {
	r := newChatRig(t)
	ins := "INSERT INTO prompt_blocks(agent, kind, fingerprint, title, panel, detail, answerable, first_seen, last_seen) VALUES(?, 'question', 'f', 't', '[]', '{}', 1, 1, 1)"
	exec(t, r.d, ins, "amy")
	exec(t, r.d, ins, "bob")
	c := r.chat()
	r.scan(c)
	exec(t, r.d, "UPDATE prompt_blocks SET answered_at = 7, answer_ok = 1, answer_label = 'Other', answer_note = 'main', answer_by = 'casey' WHERE agent = 'amy'")
	exec(t, r.d, "UPDATE prompt_blocks SET gone_at = 9")
	r.scan(c)
	answers := map[string]string{}
	for _, e := range r.rows() {
		var it map[string]any
		json.Unmarshal(e.Data, &it)
		a, _ := json.Marshal(it["answer"])
		answers[*e.Agent] = string(a)
	}
	if want := `{"at":"1970-01-01T00:00:07Z","by":"casey","label":"Other","note":"main"}`; answers["amy"] != want {
		t.Fatalf("answered in Handline: %s", answers["amy"])
	}
	if want := `{"at":"1970-01-01T00:00:09Z","outside":true}`; answers["bob"] != want {
		t.Fatalf("answered in the terminal: %s", answers["bob"])
	}
}

func TestAConversationIsToldOnceWhenItsSessionIdIsKnown(t *testing.T) {
	r := newChatRig(t)
	now := time.Now().Unix()
	ins := "INSERT INTO session_starts(agent, ts, session_id, cause, resumed, text, prompt) VALUES(?, ?, ?, ?, ?, '', '')"
	exec(t, r.d, ins, "amy", now, nil, "launch", 0) // written before the daemon, bound after it: told when it is bound
	c := r.chat()
	r.scan(c)
	exec(t, r.d, ins, "bob", now, "s1", "wake", 0)      // a structured start knows its session at once
	exec(t, r.d, ins, "bob", now, "s1", "wake", 1)      // a resume that only woke it begins nothing
	exec(t, r.d, ins, "cat", now, nil, "start", 0)      // a terminal launch waits for its hook
	exec(t, r.d, ins, "dan", now-3600, nil, "start", 0) // past the bind window: never bound
	r.scan(c)
	wantTypes(t, r.types(), "chat.transcript:bob")
	exec(t, r.d, "UPDATE session_starts SET session_id = 's2' WHERE agent = 'cat'")
	exec(t, r.d, "UPDATE session_starts SET session_id = 's3' WHERE agent = 'amy'")
	r.scan(c)
	r.scan(c)
	wantTypes(t, r.types(), "chat.transcript:bob", "chat.transcript:amy", "chat.transcript:cat")
	var it map[string]any
	json.Unmarshal(r.rows()[2].Data, &it)
	if it["kind"] != "session" || it["session"].(map[string]any)["id"] != "s2" || it["session"].(map[string]any)["cause"] != "start" {
		t.Fatalf("%v", it)
	}
	exec(t, r.d, ins, "eve", now, "s4", "settings_applied", 1) // a restart into the same conversation is told as it happens
	r.scan(c)
	r.scan(c)
	rows := r.rows()
	json.Unmarshal(rows[len(rows)-1].Data, &it)
	if rows[len(rows)-1].Type != "chat.transcript" || it["session"].(map[string]any)["resumed"] != true {
		t.Fatalf("%v", it)
	}
}

func TestAnAgentsWorkingFlagIsToldWhenItFlipsAndNotWhenItDoesNot(t *testing.T) {
	r := newChatRig(t)
	exec(t, r.d, "INSERT INTO agent_view(agent, view, updated) VALUES('bob', ?, 0)", `{"name":"bob","state":"idle"}`)
	c := r.chat()
	r.scan(c)
	exec(t, r.d, "UPDATE agent_view SET view = ? WHERE agent = 'bob'", `{"name":"bob","state":"running","state_since":"x"}`)
	r.scan(c)
	r.scan(c)
	wantTypes(t, r.types(), "chat.working:bob")
	var w map[string]any
	json.Unmarshal(r.rows()[0].Data, &w)
	if w["turn_running"] != true || w["since"] != "x" {
		t.Fatalf("%v", w)
	}
	exec(t, r.d, "UPDATE agent_view SET view = ? WHERE agent = 'bob'", `{"name":"bob","state":"running","state_since":"x","other":1}`)
	r.scan(c)
	wantTypes(t, r.types(), "chat.working:bob")
}

func TestARunStartingAndEndingIsTheMessageSaidAgainWithItsRuns(t *testing.T) {
	r := newChatRig(t)
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 100, 'bob', 'human', 'run it', 'landed', 'say')")
	started := cmdruns.Now() - 10
	panes := map[string]term.PaneRun{}
	c := r.chat()
	c.Panes = func() map[string]term.PaneRun { return panes }
	r.scan(c)
	wantTypes(t, r.types())
	run, err := cmdruns.Start(r.d, cmdruns.Block{Message: 1, N: 0}, "%4", "", "", started)
	if err != nil {
		t.Fatal(err)
	}
	panes["%4"] = term.PaneRun{Tag: run.Tag()}
	r.scan(c)
	r.scan(c)
	wantTypes(t, r.types(), "chat.message:bob")
	panes["%4"] = term.PaneRun{Tag: run.Tag(), Dead: true, Status: 1, DeadTime: started + 3}
	r.scan(c)
	r.scan(c)
	wantTypes(t, r.types(), "chat.message:bob", "chat.message:bob")
	var m struct {
		Runs []map[string]any `json:"runs"`
	}
	rows := r.rows()
	if json.Unmarshal(rows[1].Data, &m) != nil || len(m.Runs) != 1 || m.Runs[0]["state"] != "failed" || m.Runs[0]["exit"] != float64(1) || m.Runs[0]["seconds"] != float64(3) {
		t.Fatalf("%s", rows[1].Data)
	}
}

func TestARunKeptBeforeTheFirstLookIsNotNews(t *testing.T) {
	r := newChatRig(t)
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 100, 'bob', 'human', 'run it', 'landed', 'say')")
	cmdruns.Start(r.d, cmdruns.Block{Message: 1, N: 0}, "%4", "", "", cmdruns.Now())
	c := r.chat()
	r.scan(c)
	r.scan(c)
	wantTypes(t, r.types())
}

// A run of a reply item's block starting and ending sends that item again as chat.transcript, to its agent, with its runs.
func TestARunOfAReplysBlockIsTheItemSaidAgainWithItsRuns(t *testing.T) {
	r := newChatRig(t)
	exec(t, r.d, `INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('bob', 1, 's', NULL, 100, 'text', 1000, '{"text":"run make"}')`)
	// A message with the same id as the item: the reply's run is not its news.
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1000, 100, 'bob', 'human', 'same id', 'landed', 'say')")
	started := cmdruns.Now() - 10
	panes := map[string]term.PaneRun{}
	c := r.chat()
	c.Panes = func() map[string]term.PaneRun { return panes }
	r.scan(c)
	wantTypes(t, r.types())
	run, err := cmdruns.Start(r.d, cmdruns.Block{Message: 1000, N: 0, Reply: true}, "%4", "", "", started)
	if err != nil {
		t.Fatal(err)
	}
	panes["%4"] = term.PaneRun{Tag: run.Tag()}
	r.scan(c)
	wantTypes(t, r.types(), "chat.transcript:bob")
	panes["%4"] = term.PaneRun{Tag: run.Tag(), Dead: true, Status: 0, DeadTime: started + 2}
	r.scan(c)
	wantTypes(t, r.types(), "chat.transcript:bob", "chat.transcript:bob")
	var item struct {
		ID   int64            `json:"id"`
		Kind string           `json:"kind"`
		Text string           `json:"text"`
		Runs []map[string]any `json:"runs"`
	}
	rows := r.rows()
	if json.Unmarshal(rows[1].Data, &item) != nil || item.ID != 1000 || item.Kind != "reply" || item.Text != "run make" || len(item.Runs) != 1 ||
		item.Runs[0]["block"] != "r1000:0" || item.Runs[0]["state"] != "done" || item.Runs[0]["exit"] != float64(0) {
		t.Fatalf("%s", rows[1].Data)
	}
}

const bashTestSession = "11111111-2222-3333-4444-555555555555"

// bashRig is a chat watcher with one session transcript of bob's under its home.
type bashRig struct {
	*chatRig
	c   *Chat
	dir string
}

func newBashRig(t *testing.T) *bashRig {
	t.Helper()
	r := newChatRig(t)
	home := t.TempDir()
	dir := filepath.Join(home, "accounts", "alpha", ".claude", "projects", "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &bashRig{r, &Chat{Reads: &reads.Env{DB: r.d, Home: home}}, dir}
}

func (r *bashRig) write(uuid, parent, content string) {
	r.t.Helper()
	b, _ := json.Marshal(map[string]any{"type": "user", "uuid": uuid, "parentUuid": parent, "timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"message": map[string]any{"content": content}})
	f, err := os.OpenFile(filepath.Join(r.dir, bashTestSession+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		r.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		r.t.Fatal(err)
	}
}

// sent is a founder's ! line typed now; landed says whether it has landed, a second on.
func (r *bashRig) sent(id int64, landed bool) {
	r.t.Helper()
	now := time.Now().Unix()
	var at any
	if landed {
		at = now + 1
	}
	if _, err := r.d.Exec("INSERT INTO requests(id, ts, requester, target, body, state, kind, ref, typed_at, typed_session, landed_at, edge_ok) "+
		"VALUES(?, ?, 'human', 'bob', '!x', 'landed', 'say', ?, ?, ?, ?, 1)", id, now, fmt.Sprintf("r%d", id), now, bashTestSession, at); err != nil {
		r.t.Fatal(err)
	}
}

func (r *bashRig) said() []map[string]any {
	r.t.Helper()
	var out []map[string]any
	for _, e := range r.rows() {
		var m map[string]any
		if e.Type == "chat.transcript" && json.Unmarshal(e.Data, &m) == nil && m["kind"] == "bash" {
			out = append(out, m)
		}
	}
	return out
}

// A ! line's item is said when it appears and again when its output comes; the watcher stays pending only while it waits. Once settled its
// transcript is not read again.
func TestABashLineIsSaidAndSaidAgainWithItsOutput(t *testing.T) {
	r := newBashRig(t)
	r.scan(r.c) // the first look says nothing
	r.write("a", "", "<bash-input>sleep 1</bash-input>")
	r.sent(1, true)
	r.scan(r.c)
	got := r.said()
	if len(got) != 1 || got[0]["done"] != false || got[0]["request"] == nil {
		t.Fatalf("%v", got)
	}
	if !r.c.Pending() {
		t.Fatal("not pending while the output is waited for")
	}
	r.scan(r.c)
	if got := r.said(); len(got) != 1 {
		t.Fatalf("said again with nothing new: %v", got)
	}
	r.write("b", "a", "<bash-stdout>ok</bash-stdout><bash-stderr></bash-stderr>")
	r.scan(r.c)
	if got := r.said(); len(got) != 2 || got[1]["done"] != true || got[1]["stdout"] != "ok" || got[1]["id"] != got[0]["id"] {
		t.Fatalf("%v", got)
	}
	if r.c.Pending() {
		t.Fatal("pending with the output in")
	}
	r.write("t", "", "<bash-input>typed at the terminal</bash-input>")
	r.scan(r.c)
	if got := r.said(); len(got) != 2 {
		t.Fatalf("read again with every line settled: %v", got)
	}
}

// A line not landed is no reason to read, and a read for another line does not settle it: its landing brings its own.
func TestABashLineNotLandedIsReadWhenItLands(t *testing.T) {
	r := newBashRig(t)
	r.scan(r.c)
	r.sent(2, false)
	r.scan(r.c)
	if got := r.said(); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	r.write("a", "", "<bash-input>one</bash-input>")
	r.write("b", "a", "<bash-stdout>1</bash-stdout><bash-stderr></bash-stderr>")
	r.sent(1, true)
	r.scan(r.c)
	if got := r.said(); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	r.write("c", "", "<bash-input>two</bash-input>")
	r.write("d", "c", "<bash-stdout>2</bash-stdout><bash-stderr></bash-stderr>")
	if _, err := r.d.Exec("UPDATE requests SET landed_at = ? WHERE id = 2", time.Now().Unix()+1); err != nil {
		t.Fatal(err)
	}
	r.scan(r.c)
	if got := r.said(); len(got) != 2 || got[1]["command"] != "two" {
		t.Fatalf("%v", got)
	}
}

// A restart's first look takes what is there as said.
func TestTheFirstLookSaysNoBashItem(t *testing.T) {
	r := newBashRig(t)
	r.write("a", "", "<bash-input>old</bash-input>")
	r.write("b", "a", "<bash-stdout>x</bash-stdout><bash-stderr></bash-stderr>")
	r.sent(1, true)
	r.scan(r.c)
	if got := r.said(); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestAWaitingBashLineDoesNotReadTheTranscript(t *testing.T) {
	r := newBashRig(t)
	r.scan(r.c)
	r.write("a", "", "<bash-input>in the file already</bash-input>")
	r.sent(1, false)
	r.scan(r.c)
	if got := r.said(); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

// toldAbout is the says the task board queued for agent, oldest first.
func toldAbout(t *testing.T, d *sql.DB, agent string) []string {
	t.Helper()
	rows, err := d.Query("SELECT body FROM requests WHERE requester = 'task-board' AND target = ? AND kind = 'say' AND state = 'queued' ORDER BY id", agent)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		rows.Scan(&b)
		out = append(out, b)
	}
	return out
}

// A run that ends is told to the agent it ran for, once, as a queued say: the command, its exit and the window's last rows. A stopped
// run says its window closed; a run kept with no agent tells no one; neither reads rows.
func TestARunThatEndsIsToldToItsAgentWithItsExitAndLastRows(t *testing.T) {
	r := newChatRig(t)
	exec(t, r.d, "INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(1, 100, 'human', 'bob', 'run it', 'landed', 'say', 'abc1234')")
	started := cmdruns.Now() - 10
	panes := map[string]term.PaneRun{}
	var read []string
	c := r.chat()
	c.Panes = func() map[string]term.PaneRun { return panes }
	c.Tail = func(handle string, rows int) []string {
		read = append(read, fmt.Sprintf("%s/%d", handle, rows))
		return []string{"ok  \tpkg/a", "FAIL\tpkg/b"}
	}
	r.scan(c)
	failed, _ := cmdruns.Start(r.d, cmdruns.Block{Message: 1, N: 0}, "%4", "bob", "go test ./...", started)
	reply, _ := cmdruns.Start(r.d, cmdruns.Block{Message: 1000, N: 1, Reply: true}, "%5", "bob", "make", started)
	unowned, _ := cmdruns.Start(r.d, cmdruns.Block{Message: 1, N: 1}, "%6", "", "true", started)
	panes["%4"] = term.PaneRun{Tag: failed.Tag()}
	panes["%5"] = term.PaneRun{Tag: reply.Tag()}
	panes["%6"] = term.PaneRun{Tag: unowned.Tag()}
	r.scan(c)
	if told := toldAbout(t, r.d, "bob"); len(told) != 0 || len(read) != 0 {
		t.Fatalf("a running run tells nothing: %q %v", told, read)
	}
	panes["%4"] = term.PaneRun{Tag: failed.Tag(), Dead: true, Status: 1, DeadTime: started + 3}
	delete(panes, "%5") // its window was closed
	panes["%6"] = term.PaneRun{Tag: unowned.Tag(), Dead: true}
	r.scan(c)
	r.scan(c)
	told := toldAbout(t, r.d, "bob")
	want := []string{
		"The run of code block 1 of msg_abc1234 ended with exit 1 after 3s. Command: go test ./...\nLast rows of its window:\nok      pkg/a\nFAIL    pkg/b",
		"The run of your reply's code block 2 was stopped: its window was closed before the command ended. Command: make",
	}
	if strings.Join(told, "\n--\n") != strings.Join(want, "\n--\n") {
		t.Fatalf("told:\n%s", strings.Join(told, "\n--\n"))
	}
	if strings.Join(read, ",") != "%4/40" {
		t.Fatalf("the rows read: %v", read)
	}
	if n := len(toldAbout(t, r.d, "")); n != 0 {
		t.Fatalf("a run with no agent told %d", n)
	}
}
