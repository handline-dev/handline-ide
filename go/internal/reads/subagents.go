package reads

import (
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

// subagentCounts is subagents.counts: per agent and per account, how many subagents run now and how many started today
// (db.SubagentCounts). An agent or account with none is left out.
func (e *Env) subagentCounts(_ *rpc.Conn, _ map[string]any) (any, error) {
	agents, accounts, err := db.SubagentCounts(e.DB, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"agents": agents, "accounts": accounts}, nil
}
