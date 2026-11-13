package reads

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"

	"github.com/handline/handline/go/internal/cmdruns"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/attachments"
)

func TestAMessageToAStructuredAgentCarriesItsReceiptAndLandsAsItSays(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 1, 'p1', '" + bob + "', 'held', 'queued', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(2, 2, 'p1', '" + bob + "', 'written', 'queued', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(3, 3, 'p1', '" + bob + "', 'echoed', 'queued', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(4, 4, 'p1', '" + bob + "', 'lost', 'queued', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(5, 5, 'p1', '" + bob + "', 'taken back', 'withdrawn', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(6, 6, 'p1', '" + bob + "', 'to a terminal', 'queued', 'say')",
		"INSERT INTO operations(id, kind, state, result_json, agent, created_at, updated_at) VALUES('send:" + bob + ":request:1', 'send', 'queued', '{\"uuid\":\"u1\"}', '" + bob + "', 1, 1)",
		"INSERT INTO operations(id, kind, state, result_json, agent, created_at, updated_at) VALUES('send:" + bob + ":request:2', 'send', 'sent', '{\"uuid\":\"u2\"}', '" + bob + "', 1, 1)",
		"INSERT INTO operations(id, kind, state, result_json, agent, created_at, updated_at) VALUES('send:" + bob + ":request:3', 'send', 'confirmed', '{\"uuid\":\"u3\"}', '" + bob + "', 1, 1)",
		"INSERT INTO operations(id, kind, state, result_json, agent, created_at, updated_at) VALUES('send:" + bob + ":request:4', 'send', 'unknown', '{\"uuid\":\"u4\"}', '" + bob + "', 1, 1)",
		"INSERT INTO operations(id, kind, state, result_json, agent, created_at, updated_at) VALUES('send:" + bob + ":request:5', 'send', 'withdrawn', '{\"uuid\":\"u5\"}', '" + bob + "', 1, 1)",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatMessages(nil, map[string]any{"agent": bob, "scope": "mine"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct {
		Messages []struct {
			ID            int64
			Landing       string
			SendingReason any `json:"sending_reason"`
			Receipt       map[string]any
		}
	}
	json.Unmarshal(b, &out)
	type want struct{ landing, receipt string }
	wants := map[int64]want{1: {"queued", "queued"}, 2: {"typed", "sent"}, 3: {"delivered", "confirmed"}, 4: {"typed", "unknown"}, 6: {"queued", ""}}
	if len(out.Messages) != len(wants) { // the taken-back message is not drawn
		t.Fatalf("%s", b)
	}
	for _, m := range out.Messages {
		w := wants[m.ID]
		state, _ := m.Receipt["state"].(string)
		if m.Landing != w.landing || state != w.receipt || (w.receipt != "" && m.Receipt["uuid"] != "u"+string(rune('0'+m.ID))) {
			t.Errorf("#%d: landing %s receipt %v, want %s %s", m.ID, m.Landing, m.Receipt, w.landing, w.receipt)
		}
		if m.ID == 4 && m.SendingReason != "unknown" {
			t.Errorf("a lost message says why it is not delivered: %v", m.SendingReason)
		}
	}
}

func TestChatTranscriptServesAToolCallWithItsOutput(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for i, ev := range []struct{ kind, body string }{
		{"tool_use", `{"tool_use":{"id":"u1","tool":"Bash","input":{"command":"seq 1 10"}}}`},
		{"tool_result", `{"tool_result":{"tool_use_id":"u1","ok":true,"text":"1\n2\n","truncated":true,"dropped_lines":8}}`},
		{"tool_use", `{"tool_use":{"id":"u2","tool":"Bash","input":{"command":"true"}}}`},
	} {
		if _, err := e.DB.Exec(`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('`+bob+`', ?, 's', NULL, 1000, ?, NULL, ?)`, i+1, ev.kind, ev.body); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatTranscript(nil, map[string]any{"agent": bob})
	if err != nil {
		t.Fatal(err)
	}
	var tools []map[string]any
	for _, it := range got.(map[string]any)["items"].([]any) {
		if m := it.(map[string]any); m["kind"] == "tool" {
			tools = append(tools, m)
		}
	}
	if len(tools) != 2 || tools[0]["text"] != "seq 1 10" || tools[0]["output"] != "1\n2\n" || tools[0]["dropped_lines"] != int64(8) {
		t.Fatalf("%v", tools)
	}
	if _, has := tools[1]["output"]; has || tools[1]["text"] != "true" {
		t.Fatalf("a call with no result has no output: %v", tools[1])
	}
	if _, has := tools[1]["dropped_lines"]; has {
		t.Fatalf("%v", tools[1])
	}
}

func TestChatTranscriptListsTheConversationsAnAgentBeganAmongItsItems(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(1, '` + bob + `', 1000, 's1', 'launch', 0, '', '')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(2, '` + bob + `', 1001, 's1', 'wake', 1, 'kept', '')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(3, '` + bob + `', 1003, 's2', 'context_full', 0, 'the handoff', '')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(4, '` + bob + `', 1004, NULL, 'start', 0, '', '')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(5, 'ann', 1000, 's9', 'launch', 0, '', '')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt, changed, changed_lines) VALUES(6, '` + bob + `', 1005, 's2', 'settings_applied', 1, 'a record, not given to it', '', 'model changed (opus-5-5 -> sonnet-5-5)', '[{"part":"team rules","added":["- reply in Korean"],"removed":[],"text":"# Rules\n- reply in Korean"}]')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(7, '` + bob + `', 1006, 's2', 'restart', 1, '', '')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(8, '` + bob + `', 1007, 's2', 'move', 1, '', '')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(9, '` + bob + `', 1008, 's2', 'idle_close', 1, '', '')`,
		`INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt, changed) VALUES(10, '` + bob + `', 1009, 's2', 'wake', 1, '', '', 'effort changed (high -> low)')`,
		`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('` + bob + `', 1, 's1', NULL, 1002, 'tool_use', NULL, '{"tool_use":{"id":"u1","tool":"Bash","input":{"command":"true"}}}')`,
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatTranscript(nil, map[string]any{"agent": bob})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	var sessions []map[string]any
	for _, it := range got.(map[string]any)["items"].([]any) {
		m := it.(map[string]any)
		kinds = append(kinds, m["kind"].(string))
		if m["kind"] == "session" {
			sessions = append(sessions, m)
		}
	}
	// A resume that only woke a closed session is not listed, nor one whose session id is not known yet; a restart into the same
	// conversation is, and so is a wake that brought a settings change.
	if strings.Join(kinds, " ") != "session tool session session session session session" {
		t.Fatalf("%v", kinds)
	}
	restart := sessions[2]["session"].(map[string]any)
	if restart["cause"] != "settings_applied" || restart["resumed"] != true || restart["handoff"] != nil || restart["changed"] != "model changed (opus-5-5 -> sonnet-5-5)" {
		t.Fatalf("%v", restart)
	}
	if b, _ := json.Marshal(restart["changed_lines"]); string(b) != `[{"added":["- reply in Korean"],"part":"team rules","removed":[],"text":"# Rules\n- reply in Korean"}]` {
		t.Fatalf("changed_lines: %s", b)
	}
	first, second := sessions[0]["session"].(map[string]any), sessions[1]["session"].(map[string]any)
	if _, ok := second["changed_lines"]; ok || second["resumed"] != false || second["changed"] != "" {
		t.Fatalf("%v", second)
	}
	if sessions[0]["id"] != int64(1000_400_001) || first["id"] != "s1" || first["cause"] != "launch" || first["handoff"] != nil || first["started"] != "1970-01-01T00:16:40Z" {
		t.Fatalf("%v", sessions[0])
	}
	if sessions[1]["id"] != int64(1003_400_003) || second["cause"] != "context_full" || second["handoff"] != "the handoff" {
		t.Fatalf("%v", sessions[1])
	}
	before, err := e.chatTranscript(nil, map[string]any{"agent": bob, "before": json.Number(strconv.FormatInt(sessions[1]["id"].(int64), 10))})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(before.(map[string]any)["items"].([]any)); n != 2 {
		t.Fatalf("the page before a session item holds what is older: %d", n)
	}
	// chat.requests' session items say the same, from the first request on.
	if _, err := e.DB.Exec(`DELETE FROM session_starts WHERE id <> 6`); err != nil {
		t.Fatal(err)
	}
	ch := &chat{e: e}
	starts, err := ch.sessions(bob, nil)
	if err != nil || len(starts) != 1 || starts[0]["resumed"] != true || starts[0]["handoff"] != nil || starts[0]["changed"] != "model changed (opus-5-5 -> sonnet-5-5)" {
		t.Fatalf("%v %v", starts, err)
	}
}

func TestAPageCarriesAtMostTheOutputBudgetAndSaysWhatItCut(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("x", 800<<10) + "\n"
	items := []map[string]any{
		{"id": int64(1), "output": big + "tail\n", "dropped_lines": int64(3)},
		{"id": int64(2), "output": big},
		{"id": int64(3), "output": big},
		{"id": int64(4), "kind": "reply"},
	}
	fitToolOutput(items)
	total := 0
	for _, it := range items {
		if o, ok := it["output"].(string); ok {
			total += len(o)
		}
	}
	if total > toolOutputBudget {
		t.Fatalf("%d bytes", total)
	}
	if items[2]["output"] != big || items[1]["output"] != big || items[1]["dropped_lines"] != nil {
		t.Fatal("the newest items keep what fits")
	}
	if o := items[0]["output"].(string); len(o) != toolOutputBudget-2*len(big) || items[0]["dropped_lines"] != int64(3+2) {
		t.Fatalf("the oldest is cut to what is left, and the count it came with grows by the lines left out: %d %v", len(o), items[0]["dropped_lines"])
	}
}

func TestPagingBackThroughToolCallsThatShareASecondServesEachOnce(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for i, at := range []int{101, 102, 103, 104, 105, 105} { // parallel calls share a second
		body := fmt.Sprintf(`{"tool_use":{"id":"u%d","tool":"Bash","input":{"command":"c%d"}}}`, i, i)
		if _, err := e.DB.Exec(`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('`+bob+`', ?, 's', NULL, ?, 'tool_use', NULL, ?)`, i+1, at, body); err != nil {
			t.Fatal(err)
		}
	}
	var seen []string
	params := map[string]any{"agent": bob, "limit": json.Number("2")}
	for pages := 0; pages < 5; pages++ {
		got, err := e.chatTranscript(nil, params)
		if err != nil {
			t.Fatal(err)
		}
		page := got.(map[string]any)
		items := page["items"].([]any)
		var texts []string
		for _, it := range items {
			texts = append(texts, it.(map[string]any)["text"].(string))
		}
		seen = append(texts, seen...)
		if page["more"] != true {
			break
		}
		params = map[string]any{"agent": bob, "limit": json.Number("2"), "before": json.Number(strconv.FormatInt(items[0].(map[string]any)["id"].(int64), 10))}
	}
	if strings.Join(seen, " ") != "c0 c1 c2 c3 c4 c5" {
		t.Fatalf("%v", seen)
	}
}

func TestSinceServesEveryToolCallFromItsSecondUpToWhatAPageKeeps(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for i := 0; i < 60; i++ { // more than a default page of 50 calls in one second
		body := fmt.Sprintf(`{"tool_use":{"id":"u%d","tool":"Bash","input":{"command":"c%d"}}}`, i, i)
		if _, err := e.DB.Exec(`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('`+bob+`', ?, 's', NULL, 500, 'tool_use', NULL, ?)`, i+1, body); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatTranscript(nil, map[string]any{"agent": bob, "since": json.Number("500000000")})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.(map[string]any)["items"].([]any)); n != 60 {
		t.Fatalf("%d of 60 calls", n)
	}
}

func TestAMessageCarriesItsImages(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 1, 'p1', '" + bob + "', 'look', 'queued', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(2, 2, 'p1', '" + bob + "', 'plain', 'queued', 'say')",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	img := attachments.Info{ID: "00000000000000aa", Name: "a.png", Mime: "image/png", Bytes: 70}
	if err := attachments.Record(e.DB, 1, []attachments.Info{img}); err != nil {
		t.Fatal(err)
	}
	got, err := e.chatMessages(nil, map[string]any{"agent": bob, "scope": "mine"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct {
		Messages []struct {
			ID          int64
			Attachments []attachments.Info
		}
	}
	json.Unmarshal(b, &out)
	if len(out.Messages) != 2 || len(out.Messages[0].Attachments)+len(out.Messages[1].Attachments) != 1 {
		t.Fatalf("%s", b)
	}
	for _, m := range out.Messages {
		if (m.ID == 1) != (len(m.Attachments) == 1) || strings.Contains(string(b), `"attachments":null`) {
			t.Fatalf("%s", b)
		}
	}
}

func TestAnHROutcomeMessageCarriesItsProposalAndNoOtherDoes(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 1, 'p1', '" + bob + "', 'proposal aaaa0007 applied: ...', 'queued', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(2, 2, 'p1', '" + bob + "', 'plain', 'queued', 'say')",
		`INSERT INTO events(ts, agent, kind, payload, dedup_key) VALUES(1, 'p1', 'proposal_outcome', '{"proposal": 2684354567, "request": 1}', 'proposal_outcome:1')`,
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatMessages(nil, map[string]any{"agent": bob, "scope": "mine"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct{ Messages []map[string]any }
	json.Unmarshal(b, &out)
	if len(out.Messages) != 2 {
		t.Fatalf("%s", b)
	}
	for _, m := range out.Messages {
		if p, has := m["proposal"]; (m["id"] == float64(1)) != (has && p == float64(2684354567)) {
			t.Fatalf("only the outcome names its proposal: %s", b)
		}
	}
}

func TestATaskEventSaysWhatAnEditAndAHandoffChanged(t *testing.T) {
	t.Parallel()
	ev := func(kind, payload string) map[string]any {
		item, _, ok := taskItemOf(row{"id": int64(1), "ts": int64(5), "kind": kind, "payload": payload})
		if !ok {
			t.Fatalf("%s is not shown", kind)
		}
		return item["task_event"].(map[string]any)
	}
	if got := ev("edit_task", `{"task_id": 7, "owner": {"old": "a", "new": "b"}, "priority": {"old": 2, "new": 1}}`); got["change"] != "assigned to b, priority 2 -> 1" || got["event"] != "changed" {
		t.Fatalf("%v", got)
	}
	if got := ev("edit_task", `{"task_id": 7, "title": {"old": "x", "new": "y"}}`); got["change"] != "title changed" {
		t.Fatalf("%v", got)
	}
	if got := ev("task_handoff", `{"task_id": 7, "to": "bob", "note": "n"}`); got["change"] != "handed to bob" {
		t.Fatalf("%v", got)
	}
	for _, kind := range []string{"task_new", "mark_task_done", "queue_task"} {
		if _, has := ev(kind, `{"task_id": 7, "done": "ok"}`)["change"]; has {
			t.Errorf("%s names a change it has not got", kind)
		}
	}
}

func TestAPassOverManyTaskDeliveriesLoadsTheBoardOnce(t *testing.T) {
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	const n = 5
	for i := 1; i <= n; i++ {
		run(t, e, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(%d, 't%d', 'bob', 'p1', 1)", i, i))
		run(t, e, fmt.Sprintf("INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(%d, %d, 'p1', 'bob', 'do it', 'queued', 'task')", i, i))
		run(t, e, fmt.Sprintf("INSERT INTO task_requests(task_id, request_id) VALUES(%d, %d)", i, i))
	}
	pass := e.ChatRequests()
	for i := int64(1); i <= n; i++ {
		req, err := pass.Get(i)
		if err != nil || req == nil || req.Message["kind"] != "task" {
			t.Fatalf("%d: %v %v", i, req, err)
		}
	}
	if pass.ch.loads != 1 {
		t.Fatalf("the board was loaded %d times for %d task deliveries", pass.ch.loads, n)
	}
}

func TestANoticeTheDigestListedIsNotDrawnAgainBesideTheDigest(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, notice, landed_at) VALUES(1, 1, 'sys', '" + bob + "', 'task T1 now waits on T2', 'landed', 'say', 1, 1)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, notice, landed_at, delivered_at) VALUES(2, 2, 'sys', '" + bob + "', 'T3 done by x: ok', 'landed', 'say', 1, 2, 3)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, landed_at) VALUES(3, 4, 'sys', '" + bob + "', 'Result digest - 1 result since 00:00', 'landed', 'say', 4)",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatMessages(nil, map[string]any{"agent": bob, "scope": "all"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct{ Messages []struct{ ID int64 } }
	json.Unmarshal(b, &out)
	if len(out.Messages) != 2 || out.Messages[0].ID != 3 || out.Messages[1].ID != 1 {
		t.Fatalf("a notice still waiting for the digest and the digest are drawn; the digested one is not: %s", b)
	}
}

func TestARequestDeliveredButNotYetTypedIsStillDrawn(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, landed_at, delivered_at) VALUES(1, 1, 'p1', '" + bob + "', 'hello bob', 'landed', 'say', 1, 2)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, notice, landed_at, delivered_at) VALUES(2, 3, 'sys', '" + bob + "', 'T3 done by x: ok', 'landed', 'say', 1, 3, 4)",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, call := range []func(*rpc.Conn, map[string]any) (any, error){e.chatMessages, e.chatRequests} {
		got, err := call(nil, map[string]any{"agent": bob, "scope": "all"})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(got)
		var out struct {
			Messages []struct{ ID int64 }
			Items    []struct{ Message struct{ ID int64 } }
		}
		json.Unmarshal(b, &out)
		ids := []int64{}
		for _, m := range out.Messages {
			ids = append(ids, m.ID)
		}
		for _, it := range out.Items {
			ids = append(ids, it.Message.ID)
		}
		if len(ids) != 1 || ids[0] != 1 {
			t.Fatalf("the plain request is drawn and the digested notice is not: %s", b)
		}
	}
}

func TestEveryTaskEventNamesWhoDidIt(t *testing.T) {
	t.Parallel()
	for kind := range transcriptEvents {
		who := "by." + kind // the writer records the actor as the event's agent
		item, _, ok := taskItemOf(row{"id": int64(1), "ts": int64(5), "agent": who, "kind": kind, "payload": `{"task_id": 7}`})
		if !ok || item["task_event"].(map[string]any)["who"] != who {
			t.Errorf("%s: %v", kind, item)
		}
	}
}

func TestAMessageToTheFounderCarriesTheButtonsItsTaskStateAsksFor(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'bobs', '" + bob + "', 'p1', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(2, 'decide', 'p1', '" + bob + "', 1)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(3, 'not theirs', '" + bob + "', '" + bob + "', 1)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 1, '" + bob + "', 'p1', 'T1 reported', 'queued', 'say'), (2, 2, '" + bob + "', 'p1', 'T2 please decide', 'queued', 'task'), " +
			"(3, 3, '" + bob + "', 'p1', 'a question with no task', 'queued', 'say'), (4, 4, '" + bob + "', 'p1', 'T3 reported', 'queued', 'say')",
		"INSERT INTO task_requests(task_id, request_id) VALUES(1, 1), (2, 2), (3, 4)",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatMessages(nil, map[string]any{"agent": bob, "scope": "mine"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct{ Messages []map[string]any }
	json.Unmarshal(b, &out)
	keys := map[float64][]string{}
	verbs := map[string]string{}
	for _, m := range out.Messages {
		actions, _ := m["actions"].([]any)
		for _, a := range actions {
			am := a.(map[string]any)
			keys[m["id"].(float64)] = append(keys[m["id"].(float64)], am["key"].(string))
			verbs[am["key"].(string)] = am["verb"].(string)
		}
	}
	if fmt.Sprint(keys) != "map[2:[ok no]]" || verbs["ok"] != "task.done" || verbs["no"] != "task.handoff" {
		t.Fatalf("%v %v: %s", keys, verbs, b)
	}
	if _, err := e.DB.Exec("UPDATE tasks SET done_evidence = 'ok' WHERE id = 2"); err != nil {
		t.Fatal(err)
	}
	got, _ = e.chatMessages(nil, map[string]any{"agent": bob, "scope": "mine"})
	if b, _ = json.Marshal(got); strings.Contains(string(b), `"actions"`) {
		t.Fatalf("a closed task keeps its buttons: %s", b)
	}
}

// The names chat.locate is given in the tests below: msg_ and a prefix of the refs they insert.
const nameA, nameB = "msg_aaaa111", "msg_bbbb333" // local-ref-ok: test messages

func TestChatLocateTakesAMessageNameAndReturnsTheTranscriptOfItsWindow(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	for _, q := range []string{
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(1, 1000, 'p1', 'bob', 'first', 'queued', 'say', 'aaaa11112222')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(2, 2000, 'p1', 'bob', 'second', 'queued', 'say', 'bbbb33334444')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(3, 3000, 'p1', 'bob', 'third', 'queued', 'say', 'cccc55556666')",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i, at := range []int64{500, 1500, 2000, 2500, 3500} {
		body := fmt.Sprintf(`{"tool_use":{"id":"u%d","tool":"Bash","input":{"command":"at %d"}}}`, i, at)
		if _, err := e.DB.Exec(`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('bob', ?, 's', NULL, ?, 'tool_use', NULL, ?)`, i+1, at, body); err != nil {
			t.Fatal(err)
		}
	}
	tools := func(got any) []string {
		var out []string
		for _, it := range got.(map[string]any)["transcript"].([]any) {
			out = append(out, it.(map[string]any)["text"].(string))
		}
		return out
	}
	for _, name := range []string{nameB, "bbbb33334444"} {
		got, err := e.chatLocate(nil, map[string]any{"request": name})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		m := got.(map[string]any)
		if m["message"].(map[string]any)["id"] != int64(2) || len(m["messages"].([]any)) != 3 {
			t.Fatalf("%s locates message 2: %v", name, m)
		}
		// From the oldest message's second on to now, as nothing is newer than the window
		if strings.Join(tools(got), ",") != "at 1500,at 2000,at 2500,at 3500" || m["transcript_more_older"] != false || m["transcript_more_newer"] != false {
			t.Fatalf("%s: %v", name, tools(got))
		}
	}
	got, err := e.chatLocate(nil, map[string]any{"request": nameB, "limit": json.Number("1")})
	if err != nil {
		t.Fatal(err)
	}
	if m := got.(map[string]any); m["more_newer"] != true || strings.Join(tools(got), ",") != "at 2000" {
		t.Fatalf("a window of one ends with its message's second: %v", tools(got))
	}
}

func TestChatLocateKeepsAPageOfTheTranscriptAroundTheMessage(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	for _, q := range []string{
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(1, 1000, 'p1', 'bob', 'first', 'queued', 'say', 'aaaa11112222')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(2, 1200, 'p1', 'bob', 'second', 'queued', 'say', 'bbbb33334444')",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2*maxLimit; i++ {
		at := int64(1000 + i)
		body := fmt.Sprintf(`{"tool_use":{"id":"u%d","tool":"Bash","input":{"command":"at %d"}}}`, i, at)
		if _, err := e.DB.Exec(`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('bob', ?, 's', NULL, ?, 'tool_use', NULL, ?)`, i+1, at, body); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatLocate(nil, map[string]any{"request": nameA})
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	items := m["transcript"].([]any)
	first, last := items[0].(map[string]any)["text"], items[len(items)-1].(map[string]any)["text"]
	// message 1 is at the span's start, so the page starts there and the newer end is cut
	if len(items) != maxLimit || first != "at 1000" || last != fmt.Sprintf("at %d", 1000+maxLimit-1) || m["transcript_more_older"] != false || m["transcript_more_newer"] != true {
		t.Fatalf("%d items, %v .. %v, older %v newer %v", len(items), first, last, m["transcript_more_older"], m["transcript_more_newer"])
	}
	got, _ = e.chatLocate(nil, map[string]any{"request": nameB})
	m = got.(map[string]any)
	items = m["transcript"].([]any)
	if mid := items[maxLimit/2].(map[string]any)["text"]; len(items) != maxLimit || mid != "at 1200" || m["transcript_more_older"] != true || m["transcript_more_newer"] != true {
		t.Fatalf("centred on message 2: %d items, middle %v", len(items), mid)
	}
}

// More tool calls than a page before the message: the span is read around it, not from the window's start.
func TestChatLocateReadsTheToolsAroundTheMessage(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	for _, q := range []string{
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(1, 1000, 'p1', 'bob', 'first', 'queued', 'say', 'aaaa11112222')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(2, 1700, 'p1', 'bob', 'second', 'queued', 'say', 'bbbb33334444')",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 900; i++ {
		at := int64(1000 + i)
		body := fmt.Sprintf(`{"tool_use":{"id":"u%d","tool":"Bash","input":{"command":"at %d"}}}`, i, at)
		if _, err := e.DB.Exec(`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('bob', ?, 's', NULL, ?, 'tool_use', NULL, ?)`, i+1, at, body); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatLocate(nil, map[string]any{"request": nameB})
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	items := m["transcript"].([]any)
	first, last := items[0].(map[string]any)["text"], items[len(items)-1].(map[string]any)["text"]
	if len(items) != maxLimit || first != "at 1600" || last != "at 1799" || m["transcript_more_older"] != true || m["transcript_more_newer"] != true {
		t.Fatalf("%d items, %v .. %v, older %v newer %v", len(items), first, last, m["transcript_more_older"], m["transcript_more_newer"])
	}
}

// A reply item carries the runs of its command blocks; one with none has no runs, and a message's runs of the same id are not its.
func TestChatTranscriptServesAReplysRuns(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for i, item := range []int64{1000, 1002} {
		if _, err := e.DB.Exec(`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES('`+bob+`', ?, 's', NULL, 1000, 'text', ?, '{"text":"run it"}')`, i+1, item); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cmdruns.Start(e.DB, cmdruns.Block{Message: 1000, N: 0, Owner: cmdruns.ByReply}, "%4", "", "", 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := cmdruns.Start(e.DB, cmdruns.Block{Message: 1002, N: 0}, "%5", "", "", 1000); err != nil {
		t.Fatal(err)
	}
	got, err := e.chatTranscript(nil, map[string]any{"agent": bob})
	if err != nil {
		t.Fatal(err)
	}
	replies := map[int64]map[string]any{}
	for _, it := range got.(map[string]any)["items"].([]any) {
		if m := it.(map[string]any); m["kind"] == "reply" {
			replies[m["id"].(int64)] = m
		}
	}
	runs, _ := replies[1000]["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["block"] != "r1000:0" || runs[0].(map[string]any)["state"] != "running" {
		t.Fatalf("%v", replies[1000])
	}
	if _, has := replies[1002]["runs"]; has {
		t.Fatalf("message 1002's run is not reply 1002's: %v", replies[1002])
	}
}

// A turn's return that told a comment still on the task is not drawn again in the chat; a Give back with no comment, a return whose comment
// was deleted and a board line telling a comment keep theirs.
func TestTheChatLeavesOutAReturnThatToldAComment(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	runAll(t, e, "INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 1, 'p1', '"+bob+"', 'T1 is your turn again: human passed it: a', 'landed', 'task'), "+
		"(2, 2, 'p1', '"+bob+"', 'T2 is your turn again: human passed it: b', 'landed', 'task'), (3, 3, 'p1', '"+bob+"', 'plain', 'landed', 'say'), "+
		"(4, 4, 'p1', '"+bob+"', 'T3 is your turn again: human passed it: c', 'landed', 'task'), (5, 5, 'sys', '"+bob+"', 'T1 comment by the founder', 'landed', 'say')",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', '"+bob+"', 'p1', 1)",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(7, 1, 'p1', 'now', 1), (9, 1, 'p1', 'also', 5)",
		// 8 is a comment since deleted: its return shows again; 9's tell is a board line, which stays
		"INSERT INTO memo_tells(memo_id, reader, request_id) VALUES(7, '"+bob+"', 1), (8, '"+bob+"', 4), (9, '"+bob+"', 5)")
	got, err := e.chatMessages(nil, map[string]any{"agent": bob, "scope": "all"})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, m := range got.(map[string]any)["messages"].([]any) {
		ids[fmt.Sprint(m.(map[string]any)["id"])] = true
	}
	if len(ids) != 4 || ids["1"] {
		t.Fatalf("%v", ids)
	}
}

func TestAnUnblockedTaskIsACardInTheChatOfTheAgentItStartsFor(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	addAgent(t, e, "ann", "")
	addAgent(t, e, "cid", "")
	bob, ann, cid := teamtest.Key(t, e.Home, "bob"), teamtest.Key(t, e.Home, "ann"), teamtest.Key(t, e.Home, "cid")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(7, 'freed', '"+bob+"', 'lead', 1), (8, 'blocker', '"+ann+"', 'lead', 1), (9, 'turned', '"+bob+"', 'lead', 1)")
	run(t, e, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(9, 1, '"+cid+"', 'look', '"+bob+"', 1)")
	// As notifyUnblocked writes it: the closer is the event's agent.
	run(t, e, `INSERT INTO events(id, ts, agent, kind, payload) VALUES(1, 5, '`+ann+`', 'task_unblocked', '{"task_id": 7, "request_id": 3, "closed_task": 8, "after_request": 3}'),
		(2, 6, '`+ann+`', 'task_unblocked', '{"task_id": 9, "request_id": 4, "closed_task": 8, "after_request": 4}')`)
	cards := func(agent string) []map[string]any {
		got, err := e.chatTranscript(nil, map[string]any{"agent": agent})
		if err != nil {
			t.Fatal(err)
		}
		var out []map[string]any
		for _, it := range got.(map[string]any)["items"].([]any) {
			if ev, ok := it.(map[string]any)["task_event"].(map[string]any); ok {
				out = append(out, ev)
			}
		}
		return out
	}
	got := cards(bob)
	if len(got) != 1 || got[0]["event"] != "unblocked" || got[0]["task"] != int64(7) || got[0]["closed_task"] != int64(8) || got[0]["who"] != ann {
		t.Fatalf("bob: %v", got)
	}
	if c := cards(cid); len(c) != 1 || c[0]["task"] != int64(9) {
		t.Fatalf("a turn moves the card to the agent acting: %v", c)
	}
	if a := cards(ann); len(a) != 0 {
		t.Fatalf("the closer's chat has no card: %v", a)
	}
	for id, want := range map[int64]string{1: bob, 2: cid} {
		if _, agent, ok, err := e.TaskEventItem(id); err != nil || !ok || agent != want {
			t.Errorf("event %d is pushed to %q, not %q (%v)", id, agent, want, err)
		}
	}
}

// A move that landed and a person's move that could not run are one line each in the agent's own chat, and the live event is the same item.
func TestChatTranscriptShowsAnAccountMoveAndAFailedOne(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		`INSERT INTO events(id, ts, agent, kind, payload) VALUES(1, 1000, '` + bob + `', 'moved_account', '{"from":"alpha","to":"beta","trigger":"forced","note_written":true}')`,
		`INSERT INTO events(id, ts, agent, kind, payload) VALUES(2, 1001, '` + bob + `', 'moved_account', '{"from":"beta","to":"alpha","trigger":"usage"}')`,
		`INSERT INTO events(id, ts, agent, kind, payload) VALUES(3, 1002, '` + bob + `', 'move_failed', '{"to":"gamma","reason":"gamma has no free slot"}')`,
		`INSERT INTO events(id, ts, agent, kind, payload) VALUES(4, 1003, 'ann', 'moved_account', '{"from":"alpha","to":"beta","trigger":"forced"}')`,
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.chatTranscript(nil, map[string]any{"agent": bob})
	if err != nil {
		t.Fatal(err)
	}
	items := got.(map[string]any)["items"].([]any)
	want := []map[string]any{
		{"kind": "account_moved", "account_moved": map[string]any{"from": "alpha", "to": "beta", "cause": "manual", "note_written": true}},
		{"kind": "account_moved", "account_moved": map[string]any{"from": "beta", "to": "alpha", "cause": "policy", "note_written": false}},
		{"kind": "move_failed", "move_failed": map[string]any{"to": "gamma", "reason": "gamma has no free slot"}},
	}
	if len(items) != len(want) {
		t.Fatalf("%d items: %v", len(items), items)
	}
	for i, w := range want {
		m := items[i].(map[string]any)
		k := w["kind"].(string)
		if m["kind"] != k || !reflect.DeepEqual(m[k], w[k]) {
			t.Fatalf("item %d: %v, want %v", i, m, w)
		}
		live, agent, ok, err := e.TaskEventItem(int64(i + 1))
		if err != nil || !ok || agent != bob || !reflect.DeepEqual(live, m) {
			t.Fatalf("live item %d: %v %q %v %v", i, live, agent, ok, err)
		}
	}
}
