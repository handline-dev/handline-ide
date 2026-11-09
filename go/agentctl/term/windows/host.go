// Package windows is the terminal host of a Windows build. It drives no terminal yet: every call answers that, so a Windows daemon
// builds and says why it cannot start an agent instead of failing to compile.
package windows

import "github.com/handline/handline/go/agentctl/term"

// Unsupported is what every call that could not be done answers.
const Unsupported = "terminals are not supported on Windows yet"

// Host is the Windows host: never available.
type Host struct{}

func (Host) Name() string                                   { return "windows" }
func (Host) Available() bool                                { return false }
func (Host) ListTerminals() map[string]bool                 { return nil }
func (Host) ReadScreen(string, int) *term.Screen            { return nil }
func (Host) Send(string, string, bool) (bool, string)       { return false, Unsupported }
func (Host) WaitIdle(string, int, func([]string) bool) bool { return false }

func (Host) CreateTerminal(string, string, string, string) (string, string) { return "", Unsupported }
func (Host) CloseTerminal(string, bool) (bool, string)                      { return false, Unsupported }
func (Host) RespawnTerminal(string) (bool, string)                          { return false, Unsupported }

func (Host) CreateShell(string, string, string) (string, string) { return "", Unsupported }
func (Host) PaneShell(string) (string, bool)                     { return "", false }

func (Host) CreateRun(string, string, string, string, string) (string, string) {
	return "", Unsupported
}
func (Host) PaneRuns() map[string]term.PaneRun { return nil }

var (
	_ term.Host   = Host{}
	_ term.Shells = Host{}
	_ term.Runs   = Host{}
)
