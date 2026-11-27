package lifecycle

import (
	"os"
	"path/filepath"
	"slices"

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

// chatArgv is the terminal launch line turned headless.
func (l *Launcher) chatArgv(agent string, argv []string) []string {
	drv, err := l.Ports.Vendors.ForAgent(agent)
	if err != nil {
		return argv
	}
	return cli.ChatOf(drv).ChatArgv(slices.Clone(argv))
}
