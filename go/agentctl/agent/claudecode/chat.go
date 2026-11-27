package claudecode

import (
	"encoding/json"
	"slices"

	"github.com/handline/handline/go/agentctl/agent"
)

var _ agent.Chat = Driver{}

// initID names the initialize request, so the holder can drop its answer: that answer holds the account's email and organization.
const initID = "hl_init"

// ChatArgv is the stream-json launch: the same options as a terminal launch, permission asks to the host on the stream, and each
// message echoed under its uuid. The prompt is never on the line: it goes in as the first message.
func (Driver) ChatArgv(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	if i := slices.Index(argv, "--"); i >= 0 {
		argv = argv[:i]
	}
	out := []string{argv[0], "--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--permission-prompt-tool", "stdio", "--replay-user-messages"}
	return append(out, argv[1:]...)
}

func (Driver) Init() ([]byte, string) {
	b, _ := json.Marshal(map[string]any{"type": "control_request", "request_id": initID, "request": map[string]any{"subtype": "initialize"}})
	return b, `"request_id":"` + initID + `"`
}

func (Driver) UserLine(uuid, text string) []byte {
	b, _ := json.Marshal(map[string]any{"type": "user", "uuid": uuid, "parent_tool_use_id": nil,
		"message": map[string]any{"role": "user", "content": text}})
	return b
}

func (Driver) InterruptLine(id string) []byte {
	b, _ := json.Marshal(map[string]any{"type": "control_request", "request_id": id, "request": map[string]any{"subtype": "interrupt"}})
	return b
}

func (Driver) Refuse(ask agent.Event, why string) []byte {
	b, _ := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": ask.Request,
		"response": map[string]any{"behavior": "deny", "message": why}}})
	return b
}

func (Driver) Unserved(req agent.Event) []byte {
	b, _ := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": req.Request,
		"error": "not served"}})
	return b
}

// frame is every field Read may take; json drops the rest.
type frame struct {
	Type        string `json:"type"`
	Subtype     string `json:"subtype"`
	SessionID   string `json:"session_id"`
	UUID        string `json:"uuid"`
	CommandUUID string `json:"command_uuid"`
	State       string `json:"state"`
	IsReplay    bool   `json:"isReplay"`
	IsError     bool   `json:"is_error"`
	RequestID   string `json:"request_id"`
	Request     struct {
		Subtype  string `json:"subtype"`
		ToolName string `json:"tool_name"`
	} `json:"request"`
}

func (Driver) Read(line []byte) agent.Event {
	var f frame
	if json.Unmarshal(line, &f) != nil {
		return agent.Event{}
	}
	switch f.Type {
	case "system":
		if f.Subtype == "init" {
			return agent.Event{Kind: agent.EventInit, SessionID: f.SessionID}
		}
	case "command_lifecycle":
		return agent.Event{Kind: agent.EventCommand, UUID: f.CommandUUID, State: f.State}
	case "user":
		if f.IsReplay && f.UUID != "" {
			return agent.Event{Kind: agent.EventEcho, UUID: f.UUID}
		}
	case "result":
		return agent.Event{Kind: agent.EventResult, Subtype: f.Subtype, IsError: f.IsError, SessionID: f.SessionID}
	case "control_request":
		if f.Request.Subtype == "can_use_tool" {
			return agent.Event{Kind: agent.EventAsk, Request: f.RequestID, Tool: f.Request.ToolName}
		}
		return agent.Event{Kind: agent.EventRequest, Request: f.RequestID, Subtype: f.Request.Subtype}
	}
	return agent.Event{}
}
