package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/term"
)

// Driver is codex as a launch starts it (docs/design/codex-support.md, S1 findings): the TUI, one process per agent, its home the
// account's CODEX_HOME.
type Driver struct {
	Account
	agent.Host
}

var _ agent.Agent = Driver{}

func init() { agent.RegisterAgent(Vendor, func(h agent.Host) agent.Agent { return Driver{Host: h} }) }

// IsResume: a codex resume is its `resume` subcommand, which no launch passes yet.
func (Driver) IsResume(extra []string) bool { return len(extra) > 0 && extra[0] == "resume" }

// Binary is the codex on PATH, "" when there is none.
func (Driver) Binary() string {
	p, err := exec.LookPath("codex")
	if err != nil {
		return ""
	}
	return p
}

func (Driver) Screen() term.Driver { return Screen{} }

func (d Driver) Hooks() agent.Hooks { return Hooks{d.Host} }

// The approval policy is on the launch line itself; codex has no mode of Claude Code's.
func (Driver) AutoModeArgv() []string             { return nil }
func (Driver) PermissionModeArgv(string) []string { return nil }

func (Driver) LaunchEnv(string, string, string) map[string]string { return map[string]string{} }

// PromptArgv ends the options first, so a prompt that starts with a dash is the prompt.
func (Driver) PromptArgv(prompt string) []string { return []string{"--", prompt} }

// TranscriptDir: codex files sessions by date, not by worktree (SessionFile finds one by its cwd), so no folder is one worktree's.
func (Driver) TranscriptDir(string, string) (string, bool) { return "", false }

// efforts maps Handline's effort onto codex's model_reasoning_effort, which has no step above high.
var efforts = map[string]string{"low": "low", "medium": "medium", "high": "high", "xhigh": "high", "max": "high"}

// LaunchArgv is S1's launch line. --no-daemon keeps each agent off codex's shared app-server; the inline projects table trusts the
// worktree without saving it; the rules go in as developer_instructions, so nothing is written into the repo; the hooks are Handline's
// own file, so their review is bypassed. On a codex with permission profiles the sandbox reaches the daemon's socket and AllowDomains
// only; on an older one, or with WeakerNetwork, the whole network. Disallowed, Hide, NoWrite, ReadAllow and MCPArgs have no codex flag.
func (d Driver) LaunchArgv(exe string, o agent.LaunchArgs) []string {
	argv := []string{exe, "--no-daemon"}
	if o.Cwd != "" {
		argv = append(argv, "-C", o.Cwd)
	}
	if o.Model != "" {
		argv = append(argv, "-m", o.Model)
	}
	if e, ok := efforts[o.Effort]; ok {
		argv = append(argv, "-c", "model_reasoning_effort="+tomlString(e))
	}
	profile := !o.SandboxOff && !o.WeakerNetwork && d.Socket != "" && hasProfiles(exe)
	switch {
	case o.SandboxOff:
		argv = append(argv, "-s", "danger-full-access")
	case !profile:
		argv = append(argv, "-s", "workspace-write")
	}
	argv = append(argv, "-a", "on-request", "-c", "check_for_update_on_startup=false", "--dangerously-bypass-hook-trust")
	switch {
	case profile:
		argv = append(argv, d.profileArgv(o.AllowDomains)...)
	case !o.SandboxOff:
		// Without network this sandbox cannot connect to the daemon's socket either, and every hl in the session fails.
		argv = append(argv, "-c", "sandbox_workspace_write.network_access=true")
	}
	dirs := append(append([]string{}, o.ExtraDirs...), o.AllowWrite...)
	if o.CacheDir != "" && !o.SandboxOff {
		dirs = append(dirs, o.CacheDir)
	}
	for _, d := range dirs {
		argv = append(argv, "--add-dir", d)
	}
	if o.Cwd != "" {
		argv = append(argv, "-c", fmt.Sprintf("projects={%s={trust_level=\"trusted\"}}", tomlString(o.Cwd)))
	}
	if o.Preamble != "" {
		argv = append(argv, "-c", "developer_instructions="+tomlString(o.Preamble))
	}
	return argv
}

// profileArgv is the sandbox as a permission profile: workspace-write's files, the daemon's socket, and the network only through codex's
// proxy to the allowed domains. -s must stay off the line, or codex takes it over the profile.
func (d Driver) profileArgv(domains []string) []string {
	allow := func(keys []string) string {
		var kv []string
		for _, k := range keys {
			kv = append(kv, tomlString(k)+`="allow"`)
		}
		return "{" + strings.Join(kv, ",") + "}"
	}
	return []string{"-c", `default_permissions="handline"`, "-c", "features.network_proxy=true", "-c", `permissions.handline.extends=":workspace"`,
		"-c", "permissions.handline.network.enabled=true", "-c", "permissions.handline.network.domains=" + allow(domains),
		"-c", "permissions.handline.network.unix_sockets=" + allow([]string{d.Socket})}
}

// profilesSince is the first codex whose sessions were seen to hold a permission profile's socket and domains (0.157 did not).
var profilesSince = [3]int{0, 167, 0}

// versions are the versions read, by binary path and the file's size and time, so an upgrade in place is read again.
var versions sync.Map

type binaryKey struct {
	path string
	size int64
	mod  time.Time
}

var versionRE = regexp.MustCompile(`\b(\d+)\.(\d+)\.(\d+)\b`)

// versionOf is the version a binary's --version prints, ok false when it cannot be read.
var versionOf = func(exe string) ([3]int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return [3]int{}, false
	}
	return parseVersion(string(out))
}

// parseVersion is the first X.Y.Z in out.
func parseVersion(out string) (v [3]int, ok bool) {
	m := versionRE.FindStringSubmatch(out)
	if m == nil {
		return v, false
	}
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

// hasProfiles is whether exe is a codex at or past profilesSince. A version that cannot be read is not kept, and is no.
func hasProfiles(exe string) bool {
	fi, err := os.Stat(exe)
	if err != nil {
		return false
	}
	k := binaryKey{exe, fi.Size(), fi.ModTime()}
	got, ok := versions.Load(k)
	if !ok {
		v, read := versionOf(exe)
		if !read {
			return false
		}
		versions.Store(k, v)
		got = v
	}
	v := got.([3]int)
	return slices.Compare(v[:], profilesSince[:]) >= 0
}

// tomlString is s as a TOML basic string: codex parses each -c value as TOML.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
