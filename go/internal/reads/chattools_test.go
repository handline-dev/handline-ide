package reads

import (
	"encoding/json"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
)

// steps writes agent's transcript rows at second 1000, seq from 1, and reads chat.transcript's tool and thinking items.
func steps(t *testing.T, e *Env, agent string, rows ...[2]string) []map[string]any {
	t.Helper()
	for i, r := range rows {
		if _, err := e.DB.Exec(`INSERT INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES(?, ?, 's', NULL, 1000, ?, NULL, ?)`,
			agent, i+1, r[0], r[1]); err != nil {
			t.Fatal(err)
		}
	}
	return readSteps(t, e, agent)
}

func readSteps(t *testing.T, e *Env, agent string) []map[string]any {
	t.Helper()
	got, err := e.chatTranscript(nil, map[string]any{"agent": agent})
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, it := range got.(map[string]any)["items"].([]any) {
		if m := it.(map[string]any); m["kind"] == "tool" || m["kind"] == "thinking" {
			out = append(out, m)
		}
	}
	return out
}

func TestAThinkingBlockASubagentsStepsAndAnEditsDiffAreItems(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	items := steps(t, e, bob,
		[2]string{"thinking", `{"text":"weighing the two"}`},
		[2]string{"tool_use", `{"tool_use":{"id":"a1","tool":"Agent","input":{"description":"look"}}}`},
		[2]string{"thinking", `{"text":"in the subagent","parent":"a1"}`},
		[2]string{"tool_use", `{"tool_use":{"id":"e1","tool":"Edit","input":{"file_path":"a.go"}},"parent":"a1"}`},
		[2]string{"tool_result", `{"tool_result":{"tool_use_id":"e1","ok":true,"text":"updated","diff":{"hunks":[{"old_start":3,"new_start":3,"lines":["-b","+c"]}],"dropped_lines":0},"added":1,"removed":1},"parent":"a1"}`},
		[2]string{"tool_result", `{"tool_result":{"tool_use_id":"a1","ok":true,"text":"the subagent's last word"}}`},
	)
	if len(items) != 4 || items[0]["kind"] != "thinking" || items[0]["text"] != "weighing the two" {
		t.Fatalf("%v", items)
	}
	agentCall := items[1]["id"]
	if _, has := items[1]["parent"]; has || items[1]["output"] != "the subagent's last word" {
		t.Fatalf("the agent's own call, with the subagent's final word: %v", items[1])
	}
	if items[2]["kind"] != "thinking" || items[2]["parent"] != agentCall || items[3]["parent"] != agentCall {
		t.Fatalf("the subagent's steps name the call that ran it: %v", items)
	}
	b, _ := json.Marshal(items[3]["diff"])
	if string(b) != `{"dropped_lines":0,"hunks":[{"lines":["-b","+c"],"new_start":3,"old_start":3}]}` || items[3]["added"] != int64(1) || items[3]["removed"] != int64(1) {
		t.Fatalf("the edit's change: %s %v", b, items[3])
	}
	if _, has := items[1]["diff"]; has {
		t.Fatalf("a call that changed no file has no diff: %v", items[1])
	}
}

// background is a Delivery whose agents all run background work.
type background struct{ Delivery }

func (background) HasBackgroundWork(db.Q, string) (bool, error) { return true, nil }

func TestACallRunsWhileItsTurnGoesOnOrItsBackgroundJobDoes(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	e.DB.Exec(`INSERT INTO agent_view(agent, view, updated) VALUES(?, '{"state":"running","status":{"reason":"working"}}', 1)`, bob)
	e.DB.Exec(`INSERT INTO events(ts, agent, kind) VALUES(900, ?, 'Stop')`, bob)
	items := steps(t, e, bob,
		[2]string{"tool_use", `{"tool_use":{"id":"r1","tool":"Bash","input":{"command":"make"}}}`},
		[2]string{"tool_use", `{"tool_use":{"id":"r2","tool":"Bash","input":{"command":"ls"}}}`},
		[2]string{"tool_result", `{"tool_result":{"tool_use_id":"r2","ok":true,"text":"a"}}`},
		[2]string{"tool_use", `{"tool_use":{"id":"b1","tool":"Bash","input":{"command":"serve","run_in_background":true}}}`},
		[2]string{"tool_result", `{"tool_result":{"tool_use_id":"b1","ok":true,"text":"running in the background"}}`},
	)
	if items[0]["running"] != true || items[1]["running"] != nil || items[2]["running"] != nil {
		t.Fatalf("only the call with no result runs in the turn; a background call runs only while its job is listed: %v", items)
	}
	e.Delivery = background{}
	e.DB.Exec(`INSERT INTO events(ts, agent, kind, payload) VALUES(1000, ?, ?, '{"command":"serve","tool_use_id":"b1"}')`, bob, db.BackgroundStartedKind)
	if items = readSteps(t, e, bob); items[2]["running"] != true {
		t.Fatalf("a background call runs while its job is in the list: %v", items[2])
	}
	e.DB.Exec(`INSERT INTO events(ts, agent, kind) VALUES(1001, ?, 'Stop')`, bob)
	if items = readSteps(t, e, bob); items[0]["running"] != nil || items[2]["running"] != true {
		t.Fatalf("a turn that ended without the result ends its call; the background job goes on: %v", items)
	}
	e.DB.Exec(`DELETE FROM events WHERE kind = 'Stop' AND ts = 1001`)
	e.DB.Exec(`INSERT INTO events(ts, agent, kind) VALUES(1002, ?, 'UserPromptSubmit')`, bob)
	if items = readSteps(t, e, bob); items[0]["running"] != nil {
		t.Fatalf("a turn cut off with no Stop ends its call once the next prompt comes: %v", items[0])
	}
	e.DB.Exec(`DELETE FROM events WHERE kind = 'UserPromptSubmit'`)
	e.DB.Exec(`UPDATE agent_view SET view = '{"state":"idle"}' WHERE agent = ?`, bob)
	if items = readSteps(t, e, bob); items[0]["running"] != nil {
		t.Fatalf("no call runs once the turn is over: %v", items[0])
	}
}
