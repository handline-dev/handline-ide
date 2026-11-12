package codex

import (
	"slices"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
)

// The launch line is S1's: no shared app-server, the worktree trusted inline, the rules as developer_instructions in a TOML string.
func TestLaunchArgv(t *testing.T) {
	got := Driver{}.LaunchArgv("/bin/codex", agent.LaunchArgs{Cwd: "/w/a \"b\"", Model: "gpt-5.6-luna", Effort: "max",
		Preamble: "rules:\n\t\"be brief\" \\ done\x01", ExtraDirs: []string{"/w/x"}, AllowWrite: []string{"/w/y"}, CacheDir: "/c"})
	want := []string{"/bin/codex", "--no-daemon", "-C", "/w/a \"b\"", "-m", "gpt-5.6-luna", "-c", `model_reasoning_effort="high"`,
		"-s", "workspace-write", "-a", "on-request", "-c", "check_for_update_on_startup=false", "--dangerously-bypass-hook-trust",
		"-c", "sandbox_workspace_write.network_access=true", "--add-dir", "/w/x", "--add-dir", "/w/y", "--add-dir", "/c",
		"-c", `projects={"/w/a \"b\""={trust_level="trusted"}}`,
		"-c", `developer_instructions="rules:\n\t\"be brief\" \\ done\u0001"`}
	if !slices.Equal(got, want) {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
	// No model: codex's own default. Sandbox off: full access, and no cache folder to allow.
	got = Driver{}.LaunchArgv("codex", agent.LaunchArgs{Effort: "low", SandboxOff: true, CacheDir: "/c"})
	want = []string{"codex", "--no-daemon", "-c", `model_reasoning_effort="low"`, "-s", "danger-full-access", "-a", "on-request",
		"-c", "check_for_update_on_startup=false", "--dangerously-bypass-hook-trust"}
	if !slices.Equal(got, want) {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

func TestRegistersALaunchableAgent(t *testing.T) {
	ag, ok := agent.New(Vendor, agent.Host{})
	if !ok || ag.Name() != Vendor || ag.ConfigDirEnv() != "CODEX_HOME" || ag.Screen().Name() != Vendor {
		t.Fatalf("%v %v", ag, ok)
	}
	if ag.Hooks() == nil {
		t.Fatal("hooks")
	}
	if !slices.Equal(ag.PromptArgv("-x"), []string{"--", "-x"}) || ag.IsResume([]string{"--resume", "s"}) || !ag.IsResume([]string{"resume", "s"}) {
		t.Fatal("prompt or resume")
	}
}
