// Package agenthost is what the daemon tells agentctl about a Handline home: its tmux server and the variables a terminal gets, the
// terminal host config.yaml names, the daemon's socket and the hl command names.
package agenthost

import (
	"os"
	"strings"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/agentctl/term/hosts"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/yamlcompat"
)

func init() {
	agent.UserHome = paths.UserHome
	agent.RunDir = paths.QuietDir
}

// Host is the daemon of home, as an agent CLI's hooks and launch see it.
func Host(home string) agent.Host {
	return agent.Host{Home: home, Socket: paths.SocketFile(home), Commands: paths.Commands}
}

// Place is where the daemon of home keeps its terminals. Its home and tmux server go into each terminal's environment, so what runs
// there reads them and not whatever the tmux server or the daemon's own environment was started with.
func Place(home string) term.Place {
	p := term.Place{Key: home, Socket: paths.TmuxSocket(home)}
	if home != "" {
		p.Env = []string{"HANDLINE_HOME=" + home, "HANDLINE_TMUX_SOCKET=" + p.Socket}
	}
	return p
}

// Tmux is the tmux host of home, whatever host config.yaml names; on an OS without tmux it is never available.
func Tmux(home string) term.Backend { return term.ForName("tmux", Place(home)) }

// TerminalName is the host in force: HANDLINE_TERMINAL, else `terminal:` in config.yaml, else the OS's default (hosts.Default).
func TerminalName(home string) string {
	if env, _ := paths.Getenv("TERMINAL"); strings.TrimSpace(env) != "" {
		return strings.ToLower(strings.TrimSpace(env))
	}
	text, err := os.ReadFile(paths.ConfigFile(home))
	if err != nil {
		return hosts.Default
	}
	data, err := yamlcompat.Parse(string(text))
	m, _ := data.(map[string]any)
	v, _ := m["terminal"].(string)
	if err != nil || v == "" {
		return hosts.Default
	}
	return strings.ToLower(strings.TrimSpace(v))
}

// Current is the terminal host in force for home.
func Current(home string) term.Backend { return term.ForName(TerminalName(home), Place(home)) }

// Dynamic is the host in force for home, asked again on every call: the environment or config.yaml can change under a running daemon.
func Dynamic(home string) term.Backend {
	return term.Dynamic(func() term.Backend { return Current(home) })
}
