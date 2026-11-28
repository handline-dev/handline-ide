package daemon

import (
	"database/sql"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/internal/browser"
	"github.com/handline/handline/go/internal/screenpipe"
)

// screenHub is the screen pipeline: the agents' browsers on this machine, a runner's agent's over its link, and the agent told in its
// inbox when a person gives its page back.
func (d *Daemon) screenHub() *screenpipe.Hub {
	h := d.cfg.Screens
	if h == nil {
		h = &screenpipe.Hub{}
	}
	h.Open, h.Log = browser.Opener(d.cfg.Home), d.cfg.Log
	if d.nodes != nil {
		h.Stream, h.Call = d.nodes.Stream, d.nodes.Call
		h.Node = func(agent string) string {
			var t sql.NullString
			if err := d.db.QueryRow("SELECT terminal FROM agent_state WHERE agent = ?", agent).Scan(&t); err != nil {
				return ""
			}
			node, _, _ := term.OnRunner(t.String)
			return node
		}
	}
	h.Tell = func(agent, text string) {
		e := d.env()
		rid, err := e.Notice(d.db, agent, text, "screen")
		if err != nil {
			d.cfg.Log("  screen: telling %s: %v", agent, err)
			return
		}
		e.After(rid, agent)
	}
	return h
}
