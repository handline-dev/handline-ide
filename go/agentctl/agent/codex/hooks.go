package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/agentctl/agent"
)

// Wanted are the hook events Handline installs for codex: a session's start, a turn's edges (Interrupt is codex's Esc, which `hl hook`
// takes as a Stop), and the tool guard. Approvals (PermissionRequest) stay for the person at the terminal.
var Wanted = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "Interrupt"}

// The PreToolUse hook waits for a workspace's land_check (cmd/hl landCheckLimit, 25 minutes); every other one gets 10 seconds.
func hookTimeoutS(event string) int {
	if event == "PreToolUse" {
		return 25*60 + 60
	}
	return 10
}

// Hooks is the hooks.json every codex account's CODEX_HOME links to (team.sharedItems), under the program's home. Handline owns the
// whole file: a codex account's home is Handline's, never the person's ~/.codex.
type Hooks struct{ agent.Host }

var _ agent.Hooks = Hooks{}

func (h Hooks) shared() string { return filepath.Join(h.Home, "shared", ".codex") }

// Target is the shared hooks file.
func (h Hooks) Target() string { return filepath.Join(h.shared(), "hooks.json") }

func (h Hooks) EnsureShared() error { return os.MkdirAll(h.shared(), 0o755) }

type command struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type group struct {
	Hooks []command `json:"hooks"`
}

type file struct {
	Hooks map[string][]group `json:"hooks"`
}

// Missing is whether a wanted event has no `<hl> hook <event>` entry in the shared file, or its entry has another timeout.
func (h Hooks) Missing() bool {
	b, err := os.ReadFile(h.Target())
	if err != nil {
		return true
	}
	var f file
	if json.Unmarshal(b, &f) != nil {
		return true
	}
	for _, ev := range Wanted {
		gs := f.Hooks[ev]
		if len(gs) != 1 || len(gs[0].Hooks) != 1 || !strings.HasSuffix(gs[0].Hooks[0].Command, " hook "+ev) || gs[0].Hooks[0].Timeout != hookTimeoutS(ev) {
			return true
		}
	}
	return false
}

// Install writes the shared file whole: `exe hook <event>` for each wanted event. The launch line bypasses codex's hook review, so the
// file is trusted as written.
func (h Hooks) Install(exe string) error {
	f := file{Hooks: map[string][]group{}}
	for _, ev := range Wanted {
		f.Hooks[ev] = []group{{Hooks: []command{{Type: "command", Command: exe + " hook " + ev, Timeout: hookTimeoutS(ev)}}}}
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := h.EnsureShared(); err != nil {
		return err
	}
	tmp := h.Target() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, h.Target())
}

// AllowHomeSocket: the socket is reached through the launch line's network access, so there is nothing to write.
func (Hooks) AllowHomeSocket() error { return nil }
