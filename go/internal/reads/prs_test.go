package reads

import (
	"os/exec"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

func TestGithubRepoReadsHttpsAndSshRemotesAndNothingElse(t *testing.T) {
	t.Parallel()
	for url, want := range map[string]string{
		"https://github.com/handline-dev/handline.git": "handline-dev/handline",
		"https://github.com/handline-dev/handline":     "handline-dev/handline",
		"https://user@github.com/o/r.name.git\n":       "o/r.name", // local-ref-ok: userinfo in a remote is the case
		"git@github.com:o/r.git":                       "o/r",
		"ssh://git@github.com/o/r":                     "o/r",
		"https://gitlab.com/o/r.git":                   "",
		"git@github.com.evil.example:o/r.git":          "",
		"https://github.com/o":                         "",
		"/src/repo":                                    "",
		"":                                             "",
	} {
		if got := githubRepo(url); got != want {
			t.Errorf("%q: got %q, want %q", url, got, want)
		}
	}
}

// prAgent is an agent whose worktree is a git repo with the given origin ("" for none).
func prAgent(t *testing.T, e *Env, name, origin string) {
	t.Helper()
	addAgent(t, e, name, "")
	dir := t.TempDir()
	run := func(args ...string) {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q")
	if origin != "" {
		run("remote", "add", "origin", origin)
	}
	if _, err := e.DB.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES(?, 'idle', 1, ?)", teamtest.Key(t, e.Home, name), dir); err != nil {
		t.Fatal(err)
	}
}

func TestAgentGetNamesTheRepoOfItsWorktree(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	prAgent(t, e, "w", "git@github.com:o/r.git")
	got, err := e.agentGet(nil, map[string]any{"agent": teamtest.Key(t, e.Home, "w")})
	if err != nil {
		t.Fatal(err)
	}
	if wt := got.(map[string]any)["worktree"].(map[string]any); wt["repo"] != "o/r" {
		t.Fatalf("%v", wt)
	}
}

func TestAWorktreeRecordedAsRepoIdAndPathStillNamesItsRepo(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	prAgent(t, e, "w", "https://github.com/o/r.git")
	var path string
	e.DB.QueryRow("SELECT worktree FROM agent_state WHERE agent = 'w'").Scan(&path)
	e.DB.Exec("UPDATE agent_state SET worktree = ? WHERE agent = 'w'", "repo-1::"+path)
	got, err := e.agentGet(nil, map[string]any{"agent": teamtest.Key(t, e.Home, "w")})
	if err != nil {
		t.Fatal(err)
	}
	if wt := got.(map[string]any)["worktree"].(map[string]any); wt["repo"] != "o/r" {
		t.Fatalf("%v", wt)
	}
}

func TestAgentGetCarriesTheSettingsOfItsAgentFile(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "w", "")
	teamtest.Set(t, e.Home, "w", "model", "opus", "effort", "low", "weight", "light", "repo", "handline")
	w := teamtest.Key(t, e.Home, "w")
	got, err := e.agentGet(nil, map[string]any{"agent": w})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := got.(map[string]any)["settings"].(map[string]string)
	if s["model"] != "opus" || s["effort"] != "low" || s["weight"] != "light" || s["repo"] != "handline" {
		t.Fatalf("%v", s)
	}
	if _, has := s["session_account"]; has {
		t.Fatalf("a key outside the contract: %v", s)
	}
}
