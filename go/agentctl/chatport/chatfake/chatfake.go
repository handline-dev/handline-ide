// Package chatfake is a stand-in for Claude Code in Chat mode, for tests: a test binary runs it in place of its tests when an env
// variable it names says so, and starts itself under a holder.
package chatfake

import (
	"bufio"
	"encoding/json"
	"os"
)

// Run speaks Claude's stream: it answers initialize with an account, and runs each user line as a turn: queued, started, the echo,
// then (unless the text says "ask") a result. "ask" makes it ask for a tool first and wait for the answer; "long" keeps the turn open
// until the next line comes; "bg" starts two pieces of background work and meets the usage limit, and stop_task stops one. A line in the
// middle of a turn joins it.
func Run() {
	out := bufio.NewWriter(os.Stdout)
	emit := func(v map[string]any) {
		b, _ := json.Marshal(v)
		out.Write(append(b, '\n'))
		out.Flush()
	}
	sc := bufio.NewScanner(os.Stdin)
	open := false
	var cmds []string
	tasks := []map[string]any{}
	// end closes the turn as the CLI can: its result, then each of its commands completed.
	end := func(v map[string]any) {
		emit(v)
		for _, u := range cmds {
			emit(map[string]any{"type": "command_lifecycle", "command_uuid": u, "state": "completed"})
		}
		cmds = nil
	}
	for sc.Scan() {
		var m map[string]any
		_ = json.Unmarshal(sc.Bytes(), &m)
		switch m["type"] {
		case "control_request":
			req, _ := m["request"].(map[string]any)
			if req["subtype"] == "initialize" {
				emit(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": m["request_id"],
					"response": map[string]any{"account": map[string]any{"email": "someone@example.com"}}}})
				emit(map[string]any{"type": "system", "subtype": "init", "session_id": "sess-1"})
			}
			if req["subtype"] == "stop_task" {
				kept := []map[string]any{}
				for _, t := range tasks {
					if t["task_id"] != req["task_id"] {
						kept = append(kept, t)
					}
				}
				tasks = kept
				emit(map[string]any{"type": "system", "subtype": "background_tasks_changed", "tasks": tasks})
			}
			if req["subtype"] == "interrupt" && open {
				open = false
				end(map[string]any{"type": "result", "subtype": "error_during_execution", "is_error": true})
			}
		case "control_response":
			r, _ := m["response"].(map[string]any)
			inner, _ := r["response"].(map[string]any)
			emit(map[string]any{"type": "assistant", "said": inner["behavior"]})
			open = false
			end(map[string]any{"type": "result", "subtype": "success"})
		case "user":
			u, _ := m["uuid"].(string)
			text, _ := m["message"].(map[string]any)["content"].(string)
			emit(map[string]any{"type": "command_lifecycle", "command_uuid": u, "state": "queued"})
			emit(map[string]any{"type": "command_lifecycle", "command_uuid": u, "state": "started"})
			emit(map[string]any{"type": "user", "uuid": u, "isReplay": true})
			cmds = append(cmds, u)
			switch {
			case text == "ask":
				open = true
				emit(map[string]any{"type": "control_request", "request_id": "ask1", "request": map[string]any{"subtype": "can_use_tool", "tool_name": "Bash",
					"input": map[string]any{"command": "ls"}}})
				emit(map[string]any{"type": "system", "subtype": "session_state_changed", "state": "requires_action"})
			case text == "long":
				open = true
			case text == "bg":
				emit(map[string]any{"type": "system", "subtype": "session_state_changed", "state": "running"})
				tasks = []map[string]any{{"task_id": "b1", "description": "sleep 8"}, {"task_id": "b2", "description": "sleep 300"}}
				emit(map[string]any{"type": "system", "subtype": "background_tasks_changed", "tasks": tasks})
				emit(map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{"status": "rejected", "rateLimitType": "five_hour", "resetsAt": 99}})
				end(map[string]any{"type": "result", "subtype": "success", "is_error": true, "api_error_status": 429})
				emit(map[string]any{"type": "system", "subtype": "session_state_changed", "state": "idle"})
			case open: // joined the running turn, which ends now
				open = false
				end(map[string]any{"type": "result", "subtype": "success"})
			default:
				end(map[string]any{"type": "result", "subtype": "success"})
			}
		}
	}
}
