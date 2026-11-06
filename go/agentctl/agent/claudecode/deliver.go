package claudecode

import (
	"context"
	"errors"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/peerinbox"
	"github.com/handline/handline/go/agentctl/peerport"
)

// Deliver writes into the session's peer inbox (package peerport), and types the founder's own words into its terminal.
func (Driver) Deliver(env agent.DeliverEnv) agent.Port {
	p := &peerport.Port{
		Peer:        env.Peer,
		Sender:      peerinbox.New(),
		Ctx:         context.Background(),
		OnDelivered: env.OnDelivered,
		OnFailed:    env.OnFailed,
		Typed:       env.Typed,
		Typeable:    env.Typeable,
		TypeIn:      env.TypeIn,
	}
	p.Find = func(a string) (peerinbox.Session, error) {
		pl, err := env.Where(a)
		if errors.Is(err, agent.ErrNoSession) {
			return peerinbox.Session{}, peerinbox.ErrNoSession
		}
		if err != nil {
			return peerinbox.Session{}, err
		}
		return peerinbox.ByCwd(pl.Config, pl.Cwd, pl.Alive)
	}
	if env.From != nil {
		p.From = func(a, op string) peerinbox.From { return peerinbox.From{Name: env.From(a, op)} }
	}
	return p
}
