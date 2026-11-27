package compose

import (
	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/chatport"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/internal/chatterm"
	"github.com/handline/handline/go/kit/paths"
)

// chatPort is the port of the agents whose session runs in Chat mode, built once; the tick runs its reader loop.
func (p *Parts) chatPort(tell peerDelivery) *chatport.Port {
	p.chatOnce.Do(func() {
		drv := claudecode.Driver{Host: agenthost.Host(p.home)}
		env := NewPeerEnv(p.home, p.db, p.Peer, p.panePid, tell, agent.DeliverEnv{}, p.log)
		p.chat = &chatport.Port{
			Chat: func(a string) agent.Chat {
				if !p.chatSession(a) {
					return nil
				}
				return drv
			},
			Socket:      func(a string) string { return paths.ChatSocket(p.home, a) },
			OnDelivered: env.OnDelivered,
			OnFailed:    env.OnFailed,
			OnTurnEnd: func(a string, failed bool) {
				if p.log != nil {
					p.log("  chat: %s: turn ended (failed %v)", a, failed)
				}
			},
		}
	})
	return p.chat
}

// chatSession is whether agent's session runs under a Chat holder now: its socket is there. The setting says how the next one runs.
func (p *Parts) chatSession(a string) bool { return chatterm.Running(p.home, a) }
