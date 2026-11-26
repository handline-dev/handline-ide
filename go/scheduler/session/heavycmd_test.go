package session

import (
	"testing"
	"time"
)

func TestHeavyShape(t *testing.T) {
	for _, c := range []struct {
		cmd  string
		ran  time.Duration
		want string
	}{
		{"cd go && go test ./x", time.Second, "@build-test"},
		{"xcrun simctl boot 'iPhone 16'", time.Second, "@simulator"},
		{"tools/check.sh", 30 * time.Second, ""},
		{"git add -A && git commit -qm x && tools/check.sh 2>&1 | tail -20", 20 * time.Minute, "check.sh"},
		{"bash -x ./tools/check.sh origin/main", 2 * time.Minute, "check.sh"},
		{"cd /w && pnpm --filter web build:prod", 2 * time.Minute, "@build-test"},
		{"cd /w && gh run watch 123 | tail -3", 2 * time.Minute, "gh run"},
		{"rsync -a /a/ /b/", 2 * time.Minute, "rsync"},
		{"timeout 30 hl task wait T1", 5 * time.Minute, ""},
		{"sleep 120", 2 * time.Minute, ""},
		{"echo hi", 2 * time.Minute, ""},
		{"go -C go vet ./...", 2 * time.Minute, "go vet"},
		// A word after a flag may be that flag's value: it is never part of a shape.
		{"curl -u name:password https://example.com/big", 2 * time.Minute, "curl"},
		{"sshpass -p hunter2 ssh host make", 2 * time.Minute, "sshpass"},
		{"zip -P hunter2 out.zip dir", 2 * time.Minute, "zip"},
		{"git -c user.name=x status", 2 * time.Minute, "git"},
	} {
		if got := HeavyShape(c.cmd, c.ran); got != c.want {
			t.Errorf("%q after %v: %q, want %q", c.cmd, c.ran, got, c.want)
		}
	}
}

func TestABashCallIsTimedFromItsPreToolUseToItsPostToolUse(t *testing.T) {
	home := t.TempDir()
	at := time.Unix(1_700_000_000, 0)
	call := map[string]any{"tool_name": "Bash", "tool_use_id": "toolu_1", "tool_input": map[string]any{"command": "tools/check.sh"}}
	NoteBashStart(home, "w", call, at)
	if ran, ok := BashRan(home, "w", call, at.Add(90*time.Second)); !ok || ran != 90*time.Second {
		t.Errorf("%v %v", ran, ok)
	}
	if _, ok := BashRan(home, "w", call, at.Add(time.Hour)); ok {
		t.Error("read twice")
	}
	asked := map[string]any{"tool_name": "Bash", "tool_use_id": "toolu_3", "tool_input": map[string]any{"command": "git push"}}
	NoteBashStart(home, "w", asked, at)
	NoteBashStart(home, "w", call, at)
	ForgetBashStart(home, "w", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "git push"}})
	if _, ok := BashRan(home, "w", asked, at.Add(time.Minute)); ok {
		t.Error("the wait for a person's answer was timed")
	}
	if _, ok := BashRan(home, "w", call, at.Add(time.Minute)); !ok {
		t.Error("another call's start was forgotten")
	}
	bg := map[string]any{"tool_name": "Bash", "tool_use_id": "toolu_2", "tool_input": map[string]any{"command": "x", "run_in_background": true}}
	NoteBashStart(home, "w", bg, at)
	if _, ok := BashRan(home, "w", bg, at.Add(time.Second)); ok {
		t.Error("a background call is timed by its output, not its hooks")
	}
	bad := map[string]any{"tool_name": "Bash", "tool_use_id": "../x", "tool_input": map[string]any{"command": "x"}}
	NoteBashStart(home, "w", bad, at)
	if _, ok := BashRan(home, "w", bad, at); ok {
		t.Error("an id that is not a file name")
	}
}

// A call that names its own time limit and fails at it is one the limit stopped; a failure well short of the limit, a call that
// names none, and a call that ended well are not.
func TestACallThatFailsAtItsOwnTimeLimitIsToldAsTimedOut(t *testing.T) {
	home := t.TempDir()
	at := time.Unix(1_700_000_000, 0)
	call := func(id string, limitMs any) map[string]any {
		in := map[string]any{"command": "tools/check.sh origin/main"}
		if limitMs != nil {
			in["timeout"] = limitMs
		}
		return map[string]any{"tool_name": "Bash", "tool_use_id": id, "tool_input": in}
	}
	for _, c := range []struct {
		name, event string
		limitMs     any
		ran         time.Duration
		timedOut    bool
	}{
		{"at its limit", "PostToolUseFailure", 600000.0, 600 * time.Second, true},
		{"just short of it, as the hook comes a moment late or early", "PostToolUseFailure", 600000.0, 599 * time.Second, true},
		{"failed long before", "PostToolUseFailure", 600000.0, 200 * time.Second, false},
		{"ended well at its limit", "PostToolUse", 600000.0, 600 * time.Second, false},
		{"names no limit", "PostToolUseFailure", nil, 120 * time.Second, false},
	} {
		raw := call("toolu_"+c.event+c.ran.String(), c.limitMs)
		NoteBashStart(home, "w", raw, at)
		got := EndedCall(home, "w", c.event, raw, at.Add(c.ran))
		if got.TimedOut != c.timedOut || !got.Timed || got.Ran != c.ran || got.Shape != "check.sh" || got.Would != "check.sh" {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
	// A short call counts under the shape it would have, and as nothing in its agent's load.
	raw := call("toolu_short", nil)
	NoteBashStart(home, "w", raw, at)
	if got := EndedCall(home, "w", "PostToolUse", raw, at.Add(3*time.Second)); got.Shape != "" || got.Would != "check.sh" || !got.Timed {
		t.Errorf("a short call: %+v", got)
	}
	if got := EndedCall(home, "w", "PostToolUse", map[string]any{"tool_name": "Read"}, at); got != (Ended{}) {
		t.Errorf("another tool: %+v", got)
	}
}

// Every agent's open calls are read from their start notes, each with its shape and the limit it named, and no command's text.
func TestOpenStartsReadsEveryAgentsStartNotes(t *testing.T) {
	home := t.TempDir()
	at := time.Unix(1_700_000_000, 0)
	NoteBashStart(home, "a1", map[string]any{"tool_name": "Bash", "tool_use_id": "toolu_1",
		"tool_input": map[string]any{"command": "cd go && TOKEN=secret tools/check.sh", "timeout": 600000.0}}, at)
	NoteBashStart(home, "worker", map[string]any{"tool_name": "Bash", "tool_use_id": "toolu_2", "tool_input": map[string]any{"command": "sleep 30"}}, at.Add(time.Second))
	got := OpenStarts(home)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	by := map[string]OpenStart{}
	for _, s := range got {
		by[s.ID] = s
	}
	if s := by["toolu_1"]; s.Agent != "a1" || s.Shape != "check.sh" || s.Limit != 10*time.Minute || !s.At.Equal(at) {
		t.Errorf("%+v", s)
	}
	if s := by["toolu_2"]; s.Agent != "worker" || s.Shape != "" || s.Limit != 0 {
		t.Errorf("a wait has no shape and this one no limit: %+v", s)
	}
}
