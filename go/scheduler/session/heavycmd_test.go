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
