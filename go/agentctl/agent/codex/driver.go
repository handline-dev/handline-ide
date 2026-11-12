package codex

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/term"
)

// Driver is codex as a launch starts it (docs/design/codex-support.md, S1 findings): the TUI, one process per agent, its home the
// account's CODEX_HOME. Hooks, the transcript lookup and delivery are V5's; until then a codex session reports no state of its own.
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

func (Driver) Hooks() agent.Hooks { return nil }

// The approval policy is on the launch line itself; codex has no mode of Claude Code's.
func (Driver) AutoModeArgv() []string             { return nil }
func (Driver) PermissionModeArgv(string) []string { return nil }

func (Driver) LaunchEnv(string, string, string) map[string]string { return map[string]string{} }

// PromptArgv ends the options first, so a prompt that starts with a dash is the prompt.
func (Driver) PromptArgv(prompt string) []string { return []string{"--", prompt} }

func (Driver) TranscriptDir(string, string) (string, bool) { return "", false }

// Deliver: until V5 types into a codex session, its port is never live and writes nothing.
func (Driver) Deliver(agent.DeliverEnv) agent.Port { return noPort{} }

type noPort struct{}

var errNoPort = errors.New("codex sessions take no messages yet")

func (noPort) Structured(string) bool { return false }
func (noPort) Live(string) bool       { return false }
func (noPort) Send(a, _, _ string, _ delivery.Delivery) (delivery.Receipt, error) {
	return delivery.Receipt{}, fmt.Errorf("%s: %w", a, errNoPort)
}
func (noPort) Withdraw(a, _ string) (delivery.Receipt, bool, error) {
	return delivery.Receipt{}, false, fmt.Errorf("%s: %w", a, errNoPort)
}
func (noPort) Promote(a, _ string) (delivery.Receipt, error) {
	return delivery.Receipt{}, fmt.Errorf("%s: %w", a, errNoPort)
}

// efforts maps Handline's effort onto codex's model_reasoning_effort, which has no step above high.
var efforts = map[string]string{"low": "low", "medium": "medium", "high": "high", "xhigh": "high", "max": "high"}

// LaunchArgv is S1's launch line. --no-daemon keeps each agent off codex's shared app-server; the inline projects table trusts the
// worktree without saving it; the rules go in as developer_instructions, so nothing is written into the repo. Disallowed, Hide, NoWrite,
// ReadAllow, MCPArgs and AllowDomains have no codex flag: the guards that stand for them are V5's PreToolUse hooks.
func (Driver) LaunchArgv(exe string, o agent.LaunchArgs) []string {
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
	sandbox := "workspace-write"
	if o.SandboxOff {
		sandbox = "danger-full-access"
	}
	argv = append(argv, "-s", sandbox, "-a", "on-request", "-c", "check_for_update_on_startup=false")
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
