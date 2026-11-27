package claudecode

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
)

func TestChatArgvIsTheTerminalLineOnTheStreamWithoutThePrompt(t *testing.T) {
	got := Driver{}.ChatArgv([]string{"claude", "--model", "opus", "--settings", "{}", "--", "hello"})
	if got[0] != "claude" || slices.Contains(got, "hello") || slices.Contains(got, "--") {
		t.Fatal(got)
	}
	for _, want := range []string{"--print", "stream-json", "--permission-prompt-tool", "--replay-user-messages", "--model", "--settings"} {
		if !slices.Contains(got, want) {
			t.Error(want, got)
		}
	}
}

func TestReadTakesOnlyAllowedFields(t *testing.T) {
	d := Driver{}
	for line, want := range map[string]agent.Event{
		`{"type":"system","subtype":"init","session_id":"s1","email":"x"}`:                       {Kind: agent.EventInit, SessionID: "s1"},
		`{"type":"command_lifecycle","command_uuid":"u1","state":"queued"}`:                      {Kind: agent.EventCommand, UUID: "u1", State: "queued"},
		`{"type":"user","uuid":"u1","isReplay":true,"message":{"content":"hi"}}`:                 {Kind: agent.EventEcho, UUID: "u1"},
		`{"type":"user","uuid":"u2","message":{"content":"tool result"}}`:                        {},
		`{"type":"result","subtype":"error_during_execution","is_error":true,"session_id":"s1"}`: {Kind: agent.EventResult, Subtype: "error_during_execution", IsError: true, SessionID: "s1"},
		`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls"},"permission_suggestions":[{}]}}`: {
			Kind: agent.EventAsk, Request: "r1", Tool: "Bash", Input: map[string]any{"command": "ls"}},
		`{"type":"control_cancel_request","request_id":"r1"}`:                                {Kind: agent.EventCancel, Request: "r1"},
		`{"type":"control_request","request_id":"r2","request":{"subtype":"hook_callback"}}`: {Kind: agent.EventRequest, Request: "r2", Subtype: "hook_callback"},
		`{"type":"system","subtype":"session_state_changed","state":"idle"}`:                 {Kind: agent.EventState, State: "idle"},
		`{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"b1","description":"sleep 8","run_id":"r"}]}`: {
			Kind: agent.EventBackground, Tasks: []agent.Task{{ID: "b1", Description: "sleep 8"}}},
		`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`: {Kind: agent.EventBackground, Tasks: []agent.Task{}},
		`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":1795802400,"rateLimitType":"five_hour","utilization":1.01}}`: {
			Kind: agent.EventLimit, Limit: agent.Limit{Blocked: true, Window: "five_hour", ResetsAt: 1795802400}},
		`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","rateLimitType":"seven_day"}}`: {Kind: agent.EventLimit, Limit: agent.Limit{Window: "seven_day"}},
		`not json`: {},
	} {
		if got := d.Read([]byte(line)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v", line, got)
		}
	}
}

func TestTheInitAnswerIsNamedBySecret(t *testing.T) {
	d := Driver{}
	line, secret := d.Init()
	if !strings.Contains(string(line), secret) {
		t.Fatal(string(line), secret)
	}
	answer := `{"type":"control_response","response":{"subtype":"success","request_id":"hl_init","response":{"account":{}}}}`
	if !strings.Contains(answer, secret) {
		t.Fatal("the secret does not find the answer")
	}
}

func TestLinesAreJSON(t *testing.T) {
	d := Driver{}
	ask := agent.Event{Kind: agent.EventAsk, Request: "r1"}
	for _, b := range [][]byte{d.UserLine("u", "hi \"there\""), d.InterruptLine("i1"), d.StopTaskLine("s1", "b1"), d.Answer(ask, agent.AskAnswer{Message: "no"}), d.Unserved(ask)} {
		var m map[string]any
		if json.Unmarshal(b, &m) != nil || strings.Contains(string(b), "\n") {
			t.Fatal(string(b))
		}
	}
	var u struct {
		UUID    string                   `json:"uuid"`
		Message struct{ Content string } `json:"message"`
	}
	_ = json.Unmarshal(d.UserLine("u9", "hi"), &u)
	if u.UUID != "u9" || u.Message.Content != "hi" {
		t.Fatal(u)
	}
}

func TestAnAnswerAllowsWithTheInputAndRulesOrSaysNo(t *testing.T) {
	d := Driver{}
	ask := agent.Event{Kind: agent.EventAsk, Request: "r1", Input: map[string]any{"command": "ls"}}
	read := func(b []byte) map[string]any {
		var m struct {
			Response struct {
				RequestID string `json:"request_id"`
				Response  map[string]any
			}
		}
		if json.Unmarshal(b, &m) != nil || m.Response.RequestID != "r1" {
			t.Fatal(string(b))
		}
		return m.Response.Response
	}
	if got := read(d.Answer(ask, agent.AskAnswer{Allow: true})); got["behavior"] != "allow" || got["updatedInput"].(map[string]any)["command"] != "ls" ||
		got["updatedPermissions"] != nil {
		t.Fatal(got)
	}
	in := map[string]any{"questions": []any{}, "answers": map[string]any{"Pick a color": "Blue"}}
	if got := read(d.Answer(ask, agent.AskAnswer{Allow: true, Input: in})); got["updatedInput"].(map[string]any)["answers"] == nil {
		t.Fatal(got)
	}
	got := read(d.Answer(ask, agent.AskAnswer{Allow: true, Rules: []agent.Rule{{Tool: "Bash", Content: "ls"}}}))
	if b, _ := json.Marshal(got["updatedPermissions"]); string(b) != `[{"behavior":"allow","destination":"session","rules":[{"ruleContent":"ls","toolName":"Bash"}],"type":"addRules"}]` {
		t.Fatal(string(b))
	}
	if got := read(d.Answer(ask, agent.AskAnswer{Message: "no: why"})); got["behavior"] != "deny" || got["message"] != "no: why" {
		t.Fatal(got)
	}
}

func TestTheLaunchEnvTurnsTheSilentTurnReminderOffOnlyWhenAsked(t *testing.T) {
	if env := (Driver{}).LaunchEnv(agent.LaunchEnvArgs{}); len(env) != 0 {
		t.Fatal("unset leaves Claude Code's own choice:", env)
	}
	if env := (Driver{}).LaunchEnv(agent.LaunchEnvArgs{NoSilenceReminder: true}); env["CLAUDE_CODE_SILENT_TURN_REMINDER"] != "0" || len(env) != 1 {
		t.Fatal(env)
	}
}
