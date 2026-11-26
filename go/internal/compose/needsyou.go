package compose

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/agentctl/dialogs"
	"github.com/handline/handline/go/agentctl/peerinbox"
	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/internal/agentview"
	"github.com/handline/handline/go/internal/heal"
	"github.com/handline/handline/go/internal/hooks"
	"github.com/handline/handline/go/scheduler/session"
	"github.com/handline/handline/go/scheduler/worktreepaths"
)

// titles is the card's title for a need the agent's session or a hook reports, by its kind.
var titles = map[string]string{"permission": "Waiting for your OK", "question": "Asked you a question", "usage": "Usage limit: choose what to do",
	"dialog": "Waiting at a dialog"}

// movedOn is whether the agent's own turn went on past the prompt the wait file names: it asked, reported or
// committed after the prompt was raised, so the prompt was answered. A wait that names no event is left showing rather than guessed at.
func movedOn(q db.Q, agent string, w map[string]any) bool {
	after, ok := hooks.WaitID(w)
	if !ok {
		return false
	}
	last, found, err := db.LastActivityID(q, agent, session.MovedOnKinds)
	return err == nil && found && last > after
}

// liveTerminals is the terminals the host lists, nil when it could not say (or there is no host).
func (p *Parts) liveTerminals() map[string]bool {
	if p.LC == nil || p.LC.term == nil {
		return nil
	}
	return p.LC.term.LiveTerminals()
}

// switchOffer is the card for an agent parked at its wall because no account has room: it goes to the closest one if a person says so, and
// goes on waiting if nobody does. Asked live, so the account offered is one that can take it now.
func (p *Parts) switchOffer(agent string) *agentview.NeedsYou {
	if p.LC == nil || p.LC.Mover == nil {
		return nil
	}
	to, since, err := p.LC.Mover.SwitchOffer(agent)
	if err != nil || to == "" {
		return nil
	}
	return &agentview.NeedsYou{Kind: dialogs.SwitchOffer, Source: "daemon", Since: since, Account: to, Title: "No account has room to spare",
		Text: to + " is the closest, and may reach its limit again soon."}
}

// needsYou is where an agent's need of a person comes from, never the screen: its session file saying waiting (terminalSession, the one
// reader of a terminal agent's session) and the PermissionRequest hook's wait file. An agent not there to wait (no row, closed, no live
// session) needs nobody; a live session not waiting is past the prompt whatever the wait file says, as that file goes only when the call
// ran or the turn ended. The wait file counts while the row is running: a session at a prompt sends no Stop. With Config.NotifiedNeedsYou,
// a prompt hooks.Notified reports unanswered counts too. Any of them said before the agent's card was last answered ends with it.
func (p *Parts) needsYou() func(agent string) *agentview.NeedsYou {
	home, q, notified := p.home, p.db, p.notified
	return func(agent string) *agentview.NeedsYou {
		row, err := db.GetAgentState(q, agent)
		if err != nil || row == nil {
			return nil
		}
		// The daemon's own record of an agent waiting on a person: a start-up dialog it will not answer, a dialog it cannot get past, an
		// agent it could not heal. One card for the event, and it goes when the dialog closes or the trouble does. A closed agent has it
		// too: its starts for waiting work are what failed.
		if row.State == states.Stalled {
			if n := stallNeed(row); n != nil {
				return n
			}
		}
		if row.ClosedAt.Valid && row.ClosedAt.Int64 != 0 {
			return nil
		}
		if since, up := heal.CardUp(q, agent); up {
			return &agentview.NeedsYou{Kind: "stuck", Source: "daemon", Since: since, Title: "Stuck after restarts",
				Text: strings.TrimPrefix(heal.CardText, "stuck: ")}
		}
		var sess *sessionRec
		if rec, _, found := p.terminalSession(agent, p.liveTerminals()); found == sessionFound {
			sess = &rec
		}
		answered := answeredAt(q, agent)
		settled := func(at int64) bool { return at > 0 && at <= answered } // said before a person answered its card
		if sess != nil && sess.Status != "waiting" {
			return nil
		}
		w := hooks.ReadWait(home, agent)
		// Claude Code saying nobody answered comes first; a question is left to its own card.
		if since, ok := hooks.Notified(home, agent); notified && ok && !settled(since) && w["tool"] != "AskUserQuestion" {
			return &agentview.NeedsYou{Kind: "permission", Source: "notification", Since: since, Title: titles["permission"]}
		}
		if since, _ := w["since"].(float64); w != nil && row.State == states.Running && !settled(int64(since)) && !movedOn(q, agent, w) {
			tool, _ := w["tool"].(string)
			command, _ := w["command"].(string)
			kind := "permission"
			if tool == "AskUserQuestion" {
				kind = "question"
			}
			return &agentview.NeedsYou{Kind: kind, Source: "hook", Since: int64(since), Title: titles[kind],
				Text: agentview.AskLine(tool, command, worktreepaths.RecordedPath(row.Worktree.String))}
		}
		if sess != nil && !settled(sess.UpdatedAt/1000) {
			s := peerinbox.Session{Status: sess.Status, WaitingFor: sess.WaitingFor}
			kind := s.NeedsKind()
			return &agentview.NeedsYou{Kind: kind, Source: "session", Since: sess.UpdatedAt / 1000, Title: titles[kind], Text: sess.WaitingFor}
		}
		return nil
	}
}

// answeredAt is when a person last answered one of agent's prompts on its card, 0 for never. A wait the hooks or the session file
// reported before then was that prompt, so it ends with the answer and does not outlive the card.
func answeredAt(q db.Q, agent string) int64 {
	var at sql.NullInt64
	// An error reads as never: no wait ends early.
	_ = q.QueryRow("SELECT MAX(answered_at) FROM prompt_blocks WHERE agent = ? AND answer_ok = 1", agent).Scan(&at)
	return at.Int64
}

// stallNeed is the card for a stall the daemon raised itself, one of five kinds: the title and the sentence under it are fixed here, from the
// stalled row's detail, so the two paths that raise a dialog stall (the launch's and the sender's) say the same thing. nil for a stall it has no card for.
func stallNeed(row *db.AgentState) *agentview.NeedsYou {
	need := func(kind, title, text string) *agentview.NeedsYou {
		return &agentview.NeedsYou{Kind: kind, Source: "daemon", Since: row.Updated, Title: title, Text: text}
	}
	facts := row.StallFacts()
	switch verdict.ParseStuckKind(row.StallKind.String) {
	case verdict.StuckTrust:
		folder := facts["folder"]
		if folder == "" {
			folder = "its folder"
		}
		return need("trust", "Waiting for folder trust", "Claude Code is asking whether to trust the files in "+folder+". Only you can answer it, in the terminal.")
	case verdict.StuckDialog:
		ask := facts["ask"]
		if ask == "" {
			ask = "a question"
		}
		return need("dialog", "Stuck at a start-up dialog", "Claude Code is asking "+ask+". Only you can answer it, in the terminal.")
	case verdict.StuckSendDialog:
		return need("dialog", "Stuck at a dialog", "A dialog came up while it was working, so messages can't reach it: "+stallWhy(facts)+".")
	case verdict.StuckStart:
		return need("start_failed", "Won't start", fmt.Sprintf("The last %d starts failed: %s.", states.MaxFailedAttempts, stallWhy(facts)))
	case verdict.StuckWake:
		return need("no_session", "Woke but didn't start", fmt.Sprintf("It was woken %d times and no session started: %s.", states.MaxFailedAttempts, stallWhy(facts)))
	}
	return nil
}

// stallWhy is the reason the stall was raised with, "an unknown reason" when it was raised without one.
func stallWhy(facts map[string]string) string {
	if w := facts["why"]; w != "" {
		return w
	}
	return "an unknown reason"
}
