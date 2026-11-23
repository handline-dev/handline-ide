package termchan

import (
	"os"
	"strings"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/protocol/rpc"
)

// tools.install runs a missing tool's daemon-named fix in a run window stamped installStamp, so terminal.input and shell.close take it.

// installStamp is the shell stamp of an install window: no agent's name, which starts with a letter or a digit.
const installStamp = "(install)"

// installable is the tools tools.install runs the fix of (the spec's enum).
var installable = map[string]bool{"claude": true, "codex": true, "git": true}

func installTag(tool string) string { return "install:" + tool }

// installerWait follows xcode-select --install, which only asks for Apple's installer and exits: the window waits up to 30 seconds for
// the installer to open, runs on until it closes, then ends 0 if the command line tools came, else 1. The bracket keeps pgrep from
// finding this shell, whose own command line names the installer too.
const installerWait = "; i=0; until pgrep -qf \"$p\" || [ $i -ge 30 ]; do sleep 1; i=$((i+1)); done; " +
	"while pgrep -qf \"$p\"; do sleep 2; done; xcode-select -p >/dev/null"

func (e *Env) toolsInstall(_ *rpc.Conn, params map[string]any) (any, error) {
	tool, _ := params["tool"].(string)
	if !installable[tool] {
		return nil, bad("tool is claude, codex or git", "tool")
	}
	e.runStart.Lock() // one window per tool: two calls at once find the first one's
	defer e.runStart.Unlock()
	host, err := e.shellHost()
	if err != nil {
		return nil, err
	}
	runner, can := host.(term.Runs)
	if !can {
		return nil, rpc.Refused("the terminal host ("+e.Backend().Name()+") cannot run a command", "unsupported", "")
	}
	// a running install is answered again, even once its binary is on PATH; an ended one's window gives way to the new run
	var ended []string
	for h, r := range runner.PaneRuns() {
		if r.Tag != installTag(tool) {
			continue
		}
		if !r.Dead {
			return e.installing(tool, h), nil
		}
		ended = append(ended, h)
	}
	fix := e.Missing()[tool]
	if fix == "" {
		return nil, notFound(tool+" is not missing", "tool", tool)
	}
	if c, ok := host.(term.Host); ok {
		for _, h := range ended {
			c.CloseTerminal(h, true)
		}
	}
	if fix == "xcode-select --install" {
		fix = "p='Install Command Line Developer Tools[.]app'; " + fix + installerWait
	} else { // a fix ends 0 only when the tool came: a curl that failed into bash's pipe ends 0 too, and its window then closed on its error
		fix = "set -o pipefail; " + fix + " && command -v " + tool + " >/dev/null"
	}
	cwd, _ := os.UserHomeDir()                                                                        // none: the host's own default folder
	handle, why := runner.CreateRun(cwd, "install-"+tool, installStamp, installTag(tool), fix, false) // one that ends 0 closes: the tool came
	if handle == "" {
		return nil, rpc.Refused("no window was opened: "+why, "state", "")
	}
	return e.installing(tool, handle), nil
}

func (e *Env) installing(tool, handle string) map[string]any {
	return map[string]any{"tool": tool, "terminal": handle, "host": e.Backend().Name()}
}

// ToolInstall is DaemonStatus' install of a missing tool: its install window while the host keeps it, and whether its command ended.
type ToolInstall struct {
	Terminal string `json:"terminal"`
	Ended    bool   `json:"ended"`
}

// Installs is each tool's install window the host keeps, running or ended and not yet closed, by its tool: a running one over an
// ended one, else the one that ended last.
func (e *Env) Installs() map[string]ToolInstall {
	out := map[string]ToolInstall{}
	ended := map[string]int64{}
	for h, r := range e.PaneRuns() {
		tool, ok := strings.CutPrefix(r.Tag, installTag(""))
		if !ok {
			continue
		}
		if cur, seen := out[tool]; seen && (!cur.Ended || r.Dead && r.DeadTime <= ended[tool]) {
			continue
		}
		out[tool], ended[tool] = ToolInstall{Terminal: h, Ended: r.Dead}, r.DeadTime
	}
	return out
}
