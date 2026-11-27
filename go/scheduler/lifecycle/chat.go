package lifecycle

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
)

// ChatMode is whether agent's next session runs headless (Chat mode): its setting says chat and its CLI has a Chat mode.
func (l *Launcher) ChatMode(agent string) bool {
	cfg, err := l.Roster.Read(agent)
	if err != nil || team.SessionMode(cfg) != "chat" {
		return false
	}
	drv, err := l.Ports.Vendors.ForAgent(agent)
	return err == nil && cli.ChatOf(drv) != nil
}

// chatHold is the holder's part of a Chat launch, nil for a terminal one. The first prompt goes in as the first message, after the line
// that opens the session. A runner's session stays a terminal one until runners hold Chat sessions.
func (l *Launcher) chatHold(site *Site, agent, prompt string) (*Hold, error) {
	if site.Node != "" || !l.ChatMode(agent) {
		return nil, nil
	}
	drv, err := l.Ports.Vendors.ForAgent(agent)
	if err != nil {
		return nil, err
	}
	chat := cli.ChatOf(drv)
	sock := paths.ChatSocket(l.Home, agent)
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		return nil, err
	}
	first, secret := chat.Init()
	init := []string{string(first)}
	if prompt != "" {
		init = append(init, string(chat.UserLine(delivery.UUIDFor("launch:"+agent+":"+l.now().String()), prompt)))
	}
	return &Hold{Socket: sock, Init: init, Secret: secret}, nil
}

// chatEnv adds what the agent's CLI wants in its environment for a Chat session.
func (l *Launcher) chatEnv(agent string, env map[string]string) {
	drv, err := l.Ports.Vendors.ForAgent(agent)
	if err != nil {
		return
	}
	for _, kv := range cli.ChatOf(drv).Env() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
}

// addDirs are the folders the session works in beside its own: the agent's other folders, and for a Chat session the granted folders too,
// which a terminal session is given at once by /add-dir and a Chat one by being resumed with them.
func (l *Launcher) addDirs(site *Site, agent string, granted []string) []string {
	dirs := l.Roster.OtherFolders(agent)
	if site.Node != "" || !l.ChatMode(agent) {
		return dirs
	}
	for _, g := range granted {
		if fi, err := os.Stat(g); err == nil && fi.IsDir() && !slices.Contains(dirs, g) {
			dirs = append(dirs, g)
		}
	}
	return dirs
}

// chatArgv is the terminal launch line turned headless.
func (l *Launcher) chatArgv(agent string, argv []string) []string {
	drv, err := l.Ports.Vendors.ForAgent(agent)
	if err != nil {
		return argv
	}
	return cli.ChatOf(drv).ChatArgv(slices.Clone(argv))
}
