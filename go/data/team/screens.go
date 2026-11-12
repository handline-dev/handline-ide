package team

import (
	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/data/db"
)

// ScreenFor is the screen wording of each terminal of home: that of the vendor its agent runs, else fallback's (a terminal of no
// agent, or of the default vendor). It is the lookup term.New takes.
func ScreenFor(home string, q db.Q, fallback func(handle string) term.Driver) func(handle string) term.Driver {
	roster := NewRosterOn(q, home)
	return func(handle string) term.Driver {
		name, err := db.AgentOfTerminal(q, handle)
		if err != nil || name == "" {
			return fallback(handle)
		}
		cfg, err := roster.Read(name)
		if err != nil {
			return fallback(handle)
		}
		vendor, _, _ := Parts(cfg)
		if vendor == DefaultVendor {
			return fallback(handle)
		}
		if ag, ok := agent.New(vendor, agenthost.Host(home)); ok {
			return ag.Screen()
		}
		return fallback(handle)
	}
}
