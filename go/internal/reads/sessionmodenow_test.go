package reads

import (
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

// agent.get's session_mode_now is how the open session runs now, from the check agent.open_terminal makes: null with none open, chat
// while its terminal is headless, terminal otherwise, and terminal after Open terminal even while the setting in force says chat.
func TestAgentGetSaysHowTheOpenSessionRunsNow(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "w", "")
	w := teamtest.Key(t, e.Home, "w")
	headless := false
	e.Headless = func(agent, handle string) bool { return headless && agent == w && handle == "t1" }
	get := func() map[string]any {
		t.Helper()
		got, err := e.agentGet(nil, map[string]any{"agent": w})
		if err != nil {
			t.Fatal(err)
		}
		return got.(map[string]any)
	}
	if got := get(); got["session_mode_now"] != nil || got["session_open"] != false {
		t.Fatalf("no session: %v %v", got["session_mode_now"], got["session_open"])
	}
	if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, updated, terminal) VALUES(?, 'idle', 1, 't1')", w); err != nil {
		t.Fatal(err)
	}
	if got := get(); got["session_mode_now"] != "terminal" || got["session_open"] != true {
		t.Fatalf("terminal: %v", got["session_mode_now"])
	}
	headless = true
	if got := get(); got["session_mode_now"] != "chat" {
		t.Fatalf("headless: %v", got["session_mode_now"])
	}
	teamtest.Set(t, e.Home, "w", "session_mode", "chat")
	headless = false // Open terminal resumed it in its terminal
	got := get()
	if s, _ := got["settings"].(map[string]string); got["session_mode_now"] != "terminal" || s["session_mode_in_force"] != "chat" {
		t.Fatalf("after open_terminal: %v, in force %q", got["session_mode_now"], s["session_mode_in_force"])
	}
	if _, err := e.DB.Exec("UPDATE agent_state SET closed_at = 5 WHERE agent = ?", w); err != nil {
		t.Fatal(err)
	}
	if got := get(); got["session_mode_now"] != nil || got["session_open"] != false {
		t.Fatalf("closed: %v", got["session_mode_now"])
	}
	e.Headless = nil
	if _, has := get()["session_mode_now"]; has {
		t.Fatal("told without a way to tell it")
	}
}

// agent.get's cwd is the agent's worktree while it is on disk, else the home folder: Copy folder path always has a folder.
func TestAgentGetCwdIsItsWorktreeOrTheHomeFolder(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	prAgent(t, e, "w", "")
	w := teamtest.Key(t, e.Home, "w")
	var path string
	e.DB.QueryRow("SELECT worktree FROM agent_state WHERE agent = ?", w).Scan(&path)
	got, err := e.agentGet(nil, map[string]any{"agent": w})
	if err != nil || got.(map[string]any)["cwd"] != path {
		t.Fatalf("%v %v", err, got.(map[string]any)["cwd"])
	}
	if _, err := e.DB.Exec("UPDATE agent_state SET worktree = '/gone/away' WHERE agent = ?", w); err != nil {
		t.Fatal(err)
	}
	got, err = e.agentGet(nil, map[string]any{"agent": w})
	if cwd, _ := got.(map[string]any)["cwd"].(string); err != nil || cwd == "" || cwd == "/gone/away" {
		t.Fatalf("%v %q", err, cwd)
	}
}
