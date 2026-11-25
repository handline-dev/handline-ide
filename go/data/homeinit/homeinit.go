// Package homeinit sets a Handline home up: its folders, the shared settings and event hooks, Handline's own skills, the sandbox and permission entries an agent
// needs.
package homeinit

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/kit/paths"
)

// Setup makes home ready and says each thing it changed through say.
func Setup(home, hlExe string, say func(format string, args ...any)) error {
	for _, d := range []string{home, filepath.Join(home, "agents"), filepath.Join(home, "accounts")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	hooks := claudecode.Hooks{Host: agenthost.Host(home)}
	if err := hooks.EnsureShared(); err != nil {
		return err
	}
	if changed, err := InstallSkills(hooks.SkillsDir()); err != nil {
		return err
	} else if len(changed) > 0 {
		say("Handline skills installed: %s", strings.Join(changed, ", "))
	}
	if cfg := paths.ConfigFile(home); !exists(cfg) {
		_ = os.WriteFile(cfg, []byte("version: 1\n"), 0o644)
	}
	if hlExe != "" && hooks.Missing() { // no hl to point the hooks at (a daemon with a thin PATH): leave them for `hl init`
		if err := hooks.Install(hlExe); err != nil {
			return err
		}
		say("installed Handline event hooks")
	}
	changed, blockers, err := hooks.AllowHomeWrites()
	if err != nil {
		return err
	}
	if changed {
		say("sandbox: agents may write under %s", home)
	}
	for _, e := range blockers {
		say("sandbox: denyWrite '%s' holds %s and still blocks agents from it; remove it from %s if that is not what you want", e, home, hooks.Target())
	}
	if before := readFile(hooks.Target()); hooks.AllowHomeSocket() == nil && readFile(hooks.Target()) != before {
		say("sandbox: agents may connect to %s", paths.SocketFile(home))
	}
	if added, err := hooks.AllowSafeCommands(); err == nil && added {
		say("permissions: safe commands run without asking (%s)", hooks.Target())
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func readFile(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}
