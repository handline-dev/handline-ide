package codex

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
)

// The launch line is S1's: no shared app-server, the worktree trusted inline, the rules as developer_instructions in a TOML string.
func TestLaunchArgv(t *testing.T) {
	stubVersions(t, nil)
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

// A codex with permission profiles gets one in place of -s: the daemon's socket and the allowed domains, nothing else of the network.
// WeakerNetwork keeps the old line, and an older codex too.
func TestLaunchArgvHoldsTheNetworkToTheSocketAndDomains(t *testing.T) {
	dir := t.TempDir()
	newExe, oldExe := filepath.Join(dir, "new"), filepath.Join(dir, "old")
	for _, f := range []string{newExe, oldExe} {
		if err := os.WriteFile(f, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stubVersions(t, map[string][3]int{newExe: {0, 167, 2}, oldExe: {0, 157, 1}})
	d := Driver{Host: agent.Host{Socket: "/h/daemon.sock"}}
	got := d.LaunchArgv(newExe, agent.LaunchArgs{AllowDomains: []string{"github.com", "*.npmjs.org"}})
	want := []string{newExe, "--no-daemon", "-a", "on-request", "-c", "check_for_update_on_startup=false", "--dangerously-bypass-hook-trust",
		"-c", `default_permissions="handline"`, "-c", "features.network_proxy=true", "-c", `permissions.handline.extends=":workspace"`,
		"-c", "permissions.handline.network.enabled=true", "-c", `permissions.handline.network.domains={"github.com"="allow","*.npmjs.org"="allow"}`,
		"-c", `permissions.handline.network.unix_sockets={"/h/daemon.sock"="allow"}`}
	if !slices.Equal(got, want) {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
	for _, c := range []struct {
		exe string
		o   agent.LaunchArgs
		d   Driver
	}{{newExe, agent.LaunchArgs{WeakerNetwork: true}, d}, {oldExe, agent.LaunchArgs{}, d}, {newExe, agent.LaunchArgs{}, Driver{}}} {
		got := c.d.LaunchArgv(c.exe, c.o)
		if !slices.Contains(got, "workspace-write") || !slices.Contains(got, "sandbox_workspace_write.network_access=true") || slices.Contains(got, "features.network_proxy=true") {
			t.Errorf("%s %+v: %q", c.exe, c.o, got)
		}
	}
}

// stubVersions answers versionOf from vs for the test, with nothing kept from before it.
func stubVersions(t *testing.T, vs map[string][3]int) {
	was := versionOf
	t.Cleanup(func() { versionOf = was; versions.Clear() })
	versions.Clear()
	versionOf = func(exe string) ([3]int, bool) { v, ok := vs[exe]; return v, ok }
}

func TestReadsTheCodexVersion(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho codex-cli 0.167.2 '(abc1234)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := versionOf(exe); got != [3]int{0, 167, 2} || !ok {
		t.Fatalf("%v %v", got, ok)
	}
	if _, ok := versionOf(filepath.Join(t.TempDir(), "none")); ok {
		t.Fatal("no binary read as a version")
	}
}

// The version is read again once the binary changes in place, and a failed read is not kept.
func TestTheVersionFollowsTheBinary(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "codex")
	os.WriteFile(exe, []byte("v1"), 0o755)
	vs := map[string][3]int{}
	stubVersions(t, vs)
	if hasProfiles(exe) {
		t.Fatal("unreadable version counted as new")
	}
	vs[exe] = [3]int{0, 157, 1}
	if hasProfiles(exe) {
		t.Fatal("0.157")
	}
	vs[exe] = [3]int{0, 167, 0}
	if hasProfiles(exe) {
		t.Fatal("same binary: the kept version holds")
	}
	os.WriteFile(exe, []byte("v2, upgraded"), 0o755)
	if !hasProfiles(exe) {
		t.Fatal("an upgraded binary was not read again")
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
