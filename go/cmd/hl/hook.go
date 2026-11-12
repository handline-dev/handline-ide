package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/hooks"
	"github.com/handline/handline/go/kit/pyjson"
	"github.com/handline/handline/go/protocol/sessiontoken"
	"github.com/handline/handline/go/scheduler/leases"
	"github.com/handline/handline/go/scheduler/session"
)

// captured are the hook events recorded as they come (events.CAPTURED, without TeammateIdle, which is no longer kept). Notification is
// installed for permission_prompt only, and the daemon raises the needs-you card from it (-notification-needs-you).
var captured = map[string]bool{"SessionStart": true, "SessionEnd": true, "Stop": true, "StopFailure": true, "SubagentStop": true,
	"PostCompact": true, "UserPromptSubmit": true, "PermissionRequest": true, "Notification": true}

// collected are the moments the session's own status snapshot is pulled again (events.COLLECT_EVENTS): they fire on a turn's edges and
// carry the cwd the transcript is found under. SessionEnd is not one: a snapshot written as a session exits would call it fresh.
var collected = map[string]bool{"SessionStart": true, "UserPromptSubmit": true, "Stop": true, "StopFailure": true, "PostCompact": true}

// hook is `hl hook <event>` (session/events.record_from_stdin). PreToolUse runs the guard chain; PostToolUse and its failure take away the
// wait a permission prompt raised and record the event, and a PostToolUse notes a memory file used and a command started in the background;
// the captured events are recorded, and the turn-edge ones pull the session's status snapshot. It always exits 0,
// as a hook that fails must never take the session with it.
func hook(c *hlclient.Client, args []string) (int, bool) {
	event := ""
	if len(args) > 0 {
		event = args[0]
	}
	defer func() {
		if recover() != nil { // a PreToolUse hook that fails with exit 2 blocks the call: never that
			fmt.Println("{}")
			os.Exit(0)
		}
	}()
	text, _ := io.ReadAll(os.Stdin)
	var raw map[string]any
	if strings.TrimSpace(string(text)) != "" {
		if json.Unmarshal(text, &raw) != nil {
			raw = nil
		}
	}
	if ev, _ := raw["hook_event_name"].(string); ev != "" {
		event = ev // the payload's own name, as Python took it
	}
	if event == "Interrupt" {
		event = "Stop" // codex's Esc ends the turn, as Claude Code's Stop does
	}
	agent := hlclient.Agent()
	if agent == "" || sessiontoken.New(c.Home).Nested(0) {
		fmt.Println("{}")
		return 0, true
	}
	tool, _ := raw["tool_name"].(string)
	input := raw["tool_input"]
	if event == "PermissionRequest" {
		record(c, agent, event, raw)
		fmt.Println(askPermission(c, tool, input))
		return 0, true
	}
	if event != "PreToolUse" && event != "PostToolUse" && event != "PostToolUseFailure" {
		return capture(c, agent, event, raw), true
	}
	if event != "PreToolUse" {
		env := &session.Env{Home: c.Home}
		switch name := raw["tool_name"].(type) {
		case nil: // no tool named: any prompt is answered
			env.ClearWait(agent)
		case string:
			env.ClearWaitFor(agent, name, input)
		}
		record(c, agent, event, raw)
		if event == "PostToolUse" {
			_ = session.NoteMemoryUse(c.Home, agent, tool, input, time.Now().Unix())
		}
		fmt.Println("{}")
		return 0, true
	}
	why := session.Guard(tool, input)
	if why == "" {
		why = session.LeaseGuard(tool, input, func(kind string) int { return leases.Max(usage.LoadConfig(c.Home), kind) },
			func(kind string) bool { return holds(c, agent, kind) })
	}
	if why == "" && session.PushesToGit(tool, input) {
		cwd, _ := raw["cwd"].(string)
		why = landGuard(c, agent, tool, input, cwd)
	}
	if why == "" && tool == "Agent" {
		why = subagentStart(c, agent, raw)
	}
	if why == "" {
		fmt.Println("{}")
		return 0, true
	}
	fmt.Println(pyjson.PyJSON(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": why}}, true, true))
	return 0, true
}

// landCheckLimit is how long a land_check may run; the PreToolUse hook's timeout (claudecode.longHooks) is set just past it.
const landCheckLimit = 25 * time.Minute

// landGuard is the refusal of a git push to the default branch that the workspace's land_by or land_check forbids: the rules are asked
// of the daemon within 2s, and a daemon that does not answer lets the push through. land_check is run here, in the agent's own folder
// and environment, and a push waits for it.
func landGuard(c *hlclient.Client, agent, tool string, input any, cwd string) string {
	quick := &hlclient.Client{Home: c.Home, Dial: hlclient.Dialer(c.Home, 2*time.Second), As: "hook", Deadline: 2 * time.Second}
	got, err := quick.Call("workspace.rules", map[string]any{"agent": agent})
	if err != nil {
		return ""
	}
	var rules session.Land
	for _, r := range list(got, "rules") {
		v, _ := r["value"].(string)
		switch str(r, "key") {
		case "land_by":
			rules.By = v
		case "land_check":
			rules.Check = v
		}
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	git := func(dir string, args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return string(out)
	}
	check := func(dir, command string) (string, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), landCheckLimit)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = dir
		cmd.WaitDelay = 5 * time.Second // a grandchild holding the pipe open does not outlast the limit
		out, err := cmd.CombinedOutput()
		lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
		if len(lines) > 30 {
			lines = lines[len(lines)-30:]
		}
		tail := strings.Join(lines, "\n")
		if ctx.Err() != nil {
			tail += "\n(stopped after " + landCheckLimit.String() + ")"
		}
		return tail, err == nil
	}
	return session.LandGuard(tool, input, agent, cwd, rules, git, check)
}

// subagentStart asks the daemon whether a subagent may start and has it recorded, within 2s: the refusal's reason, or "" to let it
// through, as it does when the daemon does not answer.
func subagentStart(c *hlclient.Client, agent string, raw map[string]any) string {
	sid, _ := raw["session_id"].(string)
	quick := &hlclient.Client{Home: c.Home, Dial: hlclient.Dialer(c.Home, 2*time.Second), As: "hook", Deadline: 2 * time.Second}
	params := map[string]any{"agent": agent, "session_id": sid}
	if in, _ := raw["tool_input"].(map[string]any); in != nil {
		for _, k := range []string{"description", "subagent_type", "run_in_background"} {
			if v, ok := in[k]; ok {
				params[k] = v
			}
		}
	}
	res, err := quick.Call("subagent.start", params)
	if err != nil {
		return ""
	}
	why, _ := res["deny"].(string)
	return why
}

// holds is whether agent holds a lease of kind taken with --hold, asked within 2s; a daemon that does not answer says no.
func holds(c *hlclient.Client, agent, kind string) bool {
	quick := &hlclient.Client{Home: c.Home, Dial: hlclient.Dialer(c.Home, 2*time.Second), As: "hook", Deadline: 2 * time.Second}
	got, err := quick.Call("lease.get", nil)
	if err != nil {
		return false
	}
	for _, k := range list(got, "kinds") {
		if str(k, "kind") != kind {
			continue
		}
		for _, h := range list(k, "held") {
			if str(h, "agent") == agent && h["hold"] == true {
				return true
			}
		}
	}
	return false
}

// record hands the event to the daemon, which writes it down and decides what it means for the agent's state, and returns what it
// said (nil when it did not): with the daemon not answering within 2s the event goes to the spool, which only records
// (events._record), and nothing is said. Nothing here starts a daemon.
func record(c *hlclient.Client, agent, event string, raw map[string]any) map[string]any {
	if raw == nil {
		raw = map[string]any{}
	}
	rid, at := newRequestID(), time.Now().Unix()
	quick := &hlclient.Client{Home: c.Home, Dial: hlclient.Dialer(c.Home, 2*time.Second), As: "hook", Deadline: 2 * time.Second}
	res, err := quick.Call("hook.record", map[string]any{"agent": agent, "kind": event, "at": at, "data": raw, "request_id": rid})
	if err != nil {
		_ = hooks.AppendSpool(c.Home, agent, event, raw, rid, at)
	}
	return res
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// permissionWaitS bounds the permission.ask call: the daemon holds it up to an hour (prompts.HookWaitS), and a little more covers the
// reply's way back. The hook's own timeout in settings is longer still.
const permissionWaitS = 3600 + 30

// askPermission is the PermissionRequest hook's output: the founder's answer through permission.ask, or {} when there is none (the daemon
// is down, the hour passed, or the input is not an object), and the agent's own dialog shows in its terminal. An AskUserQuestion is
// never asked: an allow would carry no answers.
func askPermission(c *hlclient.Client, tool string, input any) string {
	in, ok := input.(map[string]any)
	if tool == "" || tool == "AskUserQuestion" || !ok {
		return "{}"
	}
	waiting := *c
	waiting.Deadline = permissionWaitS * time.Second
	res, err := waiting.Call("permission.ask", map[string]any{"tool": tool, "input": in})
	behavior, _ := res["behavior"].(string)
	if err != nil || behavior == "" {
		return "{}"
	}
	decision := map[string]any{"behavior": behavior}
	if msg, _ := res["message"].(string); behavior == "deny" && msg != "" {
		decision["message"] = msg
	}
	return pyjson.PyJSON(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PermissionRequest", "decision": decision}}, true, true)
}

// capture is the rest of record_from_stdin: the record of an event that is kept and the status snapshot on a turn's edge. Anything else is
// answered and left alone.
func capture(c *hlclient.Client, agent, event string, raw map[string]any) int {
	if !captured[event] {
		fmt.Println("{}")
		return 0
	}
	// Before the record: the agent's view the daemon writes for this event reads the snapshot.
	if collected[event] {
		cwd, _ := raw["cwd"].(string)
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		sid, _ := raw["session_id"].(string)
		usage.CollectSnapshot(c.Home, agent, cwd, sid, time.Now().Unix())
	}
	res := record(c, agent, event, raw)
	out := "{}"
	if nudge, _ := res["nudge"].(string); nudge != "" { // the daemon decides, before it records the Stop; no answer, no nudge
		out = pyjson.PyJSON(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "Stop", "additionalContext": nudge}}, true, true)
	}
	fmt.Println(out)
	return 0
}
