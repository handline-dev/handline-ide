package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/internal/browser"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/hooks"
	"github.com/handline/handline/go/kit/pyjson"
	"github.com/handline/handline/go/protocol/sessiontoken"
	"github.com/handline/handline/go/scheduler/session"
)

// captured are the hook events recorded as they come (not TeammateIdle, which is no longer kept). Notification is
// installed for permission_prompt only, and the daemon raises the needs-you card from it (-notification-needs-you).
var captured = map[string]bool{"SessionStart": true, "SessionEnd": true, "Stop": true, "StopFailure": true, "SubagentStop": true,
	"PostCompact": true, "UserPromptSubmit": true, "PermissionRequest": true, "Notification": true}

// collected are the moments the session's own status snapshot is pulled again: they fire on a turn's edges and
// carry the cwd the transcript is found under. SessionEnd is not one: a snapshot written as a session exits would call it fresh.
var collected = map[string]bool{"SessionStart": true, "UserPromptSubmit": true, "Stop": true, "StopFailure": true, "PostCompact": true}

// hook is `hl hook <event>`, the event on stdin. PreToolUse runs the guard chain; PostToolUse and its failure take away the
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
	if event == "git-commit" {
		gitCommit(c)
		return 0, true
	}
	text, _ := io.ReadAll(os.Stdin)
	var raw map[string]any
	if strings.TrimSpace(string(text)) != "" {
		if json.Unmarshal(text, &raw) != nil {
			raw = nil
		}
	}
	if ev, _ := raw["hook_event_name"].(string); ev != "" {
		event = ev // the payload's own name wins over the argument
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
		session.ForgetBashStart(c.Home, agent, raw)
		record(c, agent, event, raw)
		fmt.Println(askPermission(c, tool, input))
		return 0, true
	}
	if event != "PreToolUse" && event != "PostToolUse" && event != "PostToolUseFailure" {
		capture(c, agent, event, raw)
		return 0, true
	}
	if event != "PreToolUse" {
		env := &session.Env{Home: c.Home}
		switch name := raw["tool_name"].(type) {
		case nil: // no tool named: any prompt is answered
			env.ClearWait(agent)
		case string:
			env.ClearWaitFor(agent, name, input)
		}
		if shape, ran := session.HeavyCall(c.Home, agent, raw, time.Now()); shape != "" {
			raw[hooks.Heavy] = map[string]any{"shape": shape, "seconds": int64(ran.Round(time.Second) / time.Second)}
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
		why = session.RunGuard(tool, input, sharedRuns(c, agent, tool), func(kind string) bool { return holds(c, agent, kind) })
	}
	if why == "" && session.PushesToGit(tool, input) {
		cwd, _ := raw["cwd"].(string)
		why = landGuard(c, agent, tool, input, cwd)
	}
	if why == "" && session.RunsScreencapture(tool, input) && screenViaHandline(c, agent) {
		why = session.ScreencaptureWhy
	}
	if why == "" && tool == "Agent" {
		why = subagentStart(c, agent, raw)
	}
	if why == "" {
		session.NoteBashStart(c.Home, agent, raw, time.Now())
		fmt.Println("{}")
		return 0, true
	}
	fmt.Println(pyjson.PyJSON(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": why}}, true, true))
	return 0, true
}

// gitCommit is the post-commit hook of a worktree a runner made (runner.HooksDir): the runner pushes the branch. Git runs the hook at the
// worktree's top, and its answer is not waited on past a few seconds: a commit never waits on the push.
func gitCommit(c *hlclient.Client) {
	folder, err := os.Getwd()
	if err != nil {
		return
	}
	quick := &hlclient.Client{Home: c.Home, Dial: hlclient.Dialer(c.Home, 2*time.Second), As: "hook", Deadline: 5 * time.Second}
	quick.Call("runner.committed", map[string]any{"folder": folder})
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

// screenViaHandline is whether the daemon says a shell screencapture is refused, asked within 2s: a daemon that does not answer lets it go.
func screenViaHandline(c *hlclient.Client, agent string) bool {
	quick := &hlclient.Client{Home: c.Home, Dial: hlclient.Dialer(c.Home, 2*time.Second), As: "hook", Deadline: 2 * time.Second}
	got, err := quick.Call("screen.policy", map[string]any{"agent": agent})
	via, _ := got["via"].(bool)
	return err == nil && via
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

// sharedRuns are the shared runs agent takes turns at, asked within 2s for a Bash call only; a daemon that does not answer says none.
func sharedRuns(c *hlclient.Client, agent, tool string) []session.Run {
	if tool != "Bash" {
		return nil
	}
	quick := &hlclient.Client{Home: c.Home, Dial: hlclient.Dialer(c.Home, 2*time.Second), As: "hook", Deadline: 2 * time.Second}
	got, err := quick.Call("run.list", map[string]any{"agent": agent})
	if err != nil {
		return nil
	}
	var out []session.Run
	for _, r := range list(got, "runs") {
		run := session.Run{Kind: str(r, "kind"), Name: str(r, "name"), AtOnce: int(num(r, "at_once")), SetBy: str(r, "set_by"), Why: str(r, "why")}
		cmds, _ := r["cmds"].([]any)
		for _, x := range cmds {
			if s, ok := x.(string); ok {
				run.Cmds = append(run.Cmds, s)
			}
		}
		out = append(out, run)
	}
	return out
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
// it, and nothing is said. Nothing here starts a daemon.
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
// never asked: an allow would carry no answers. The question lives as long as the call: when the connection breaks before a reply (the
// daemon restarted), the same question is asked again of the daemon that comes back, within the same hour, so its card returns.
// An allow that carries a rule (an Always allow) applies it to the running session, which asks no more for it.
func askPermission(c *hlclient.Client, tool string, input any) string {
	in, ok := input.(map[string]any)
	if tool == "" || tool == "AskUserQuestion" || !ok {
		return "{}"
	}
	end := time.Now().Add(permissionWaitS * time.Second)
	var res map[string]any
	for broke := false; ; {
		waiting := *c
		if waiting.Deadline = time.Until(end); waiting.Deadline <= 0 {
			return "{}"
		}
		var err error
		res, err = waiting.Call("permission.ask", map[string]any{"tool": tool, "input": in})
		if err == nil {
			break
		}
		var down *hlclient.DaemonDown
		// Down after a break is the daemon between its stop and its start; down from the first call is a daemon that is not there.
		if !brokenOff(err) && !(broke && errors.As(err, &down)) {
			return "{}"
		}
		broke = true
		if !daemonBack(c, end) {
			return "{}"
		}
	}
	behavior, _ := res["behavior"].(string)
	if behavior == "" {
		return "{}"
	}
	decision := map[string]any{"behavior": behavior}
	if msg, _ := res["message"].(string); behavior == "deny" && msg != "" {
		decision["message"] = msg
	}
	if r, _ := res["rule"].(map[string]any); behavior == "allow" && r != nil {
		decision["updatedPermissions"] = []any{map[string]any{"type": "addRules", "behavior": "allow", "destination": "session",
			"rules": []any{map[string]any{"toolName": r["tool"], "ruleContent": r["content"]}}}}
	}
	return pyjson.PyJSON(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PermissionRequest", "decision": decision}}, true, true)
}

// brokenOff is whether a call ended because the daemon dropped its connection with no reply: closed early, or reset. A refusal, a daemon
// that is not there, the call's own deadline or a reply that cannot be read end the question instead.
func brokenOff(err error) bool {
	var netErr net.Error
	return errors.Is(err, hlclient.ErrNoReply) || (errors.As(err, &netErr) && !netErr.Timeout())
}

// daemonBack waits for the daemon to take connections again, false when end comes first. It waits a second before each try, as a
// question queued behind another waits in the daemon (prompts.AskPermission): a stopping daemon still accepts for a moment.
func daemonBack(c *hlclient.Client, end time.Time) bool {
	for {
		time.Sleep(time.Second)
		if !time.Now().Before(end) {
			return false
		}
		if conn, err := c.Connect(); err == nil {
			conn.Close()
			return true
		}
	}
}

// capture is the rest of hook: the record of an event that is kept and the status snapshot on a turn's edge. Anything else is
// answered and left alone.
func capture(c *hlclient.Client, agent, event string, raw map[string]any) {
	if !captured[event] {
		fmt.Println("{}")
		return
	}
	// The agent's browser closes at its session's end, and one a dead session left at the next start.
	if source, _ := raw["source"].(string); event == "SessionEnd" || (event == "SessionStart" && source == "startup") {
		browser.Close(browserDir(c.Home, agent))
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
}
