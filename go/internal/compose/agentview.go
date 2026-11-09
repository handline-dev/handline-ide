package compose

import (
	"sort"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/agentview"
	"github.com/handline/handline/go/internal/prompts"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/lifecycle"
	"github.com/handline/handline/go/scheduler/session"
	"github.com/handline/handline/go/scheduler/tick"
	"github.com/handline/handline/go/taskboard/handoff"
)

// rowSessions is what the view knows of the sessions whose rows this daemon writes: with terminals, the terminal agents, whose session is
// the one their session file shows (a status busy is a turn running). Where the hooks have said (turns), they answer busy first: the
// session file still says busy when the Stop hook arrives, so a view written then would stay running.
type rowSessions struct {
	p         *Parts
	terminals bool
	turns     *turns.Tracker
}

func (r rowSessions) Structured(string) bool { return r.terminals }

func (r rowSessions) Alive(agent string) bool {
	if !r.terminals {
		return false
	}
	_, _, found := r.p.terminalSession(agent, r.p.liveTerminals())
	return found != sessionNone // a session file that cannot be read under a live pid is a session that runs
}

func (r rowSessions) Busy(agent string) bool {
	if !r.terminals {
		return false
	}
	in := verdict.Input{}
	if r.turns != nil {
		switch r.turns.State(agent) {
		case turns.Working:
			in.Turn = verdict.TurnWorking
		case turns.Idle:
			in.Turn = verdict.TurnIdle
		}
	}
	if rec, _, found := r.p.terminalSession(agent, r.p.liveTerminals()); found == sessionFound {
		in.Status = rec.Status
	}
	return verdict.Busy(verdict.AgentState(in))
}

// agentView builds the writer of agent_view rows: those of the terminal agents when terminals is on, and it drops the row of an agent
// that is gone.
func (p *Parts) agentView(cfg Config, terminals bool, tu tickUsage) *agentview.Env {
	lc, q, home := p.LC, p.db, cfg.Home
	return &agentview.Env{
		PaneRuns: p.paneRuns,
		Q:        q, Home: home,
		Sessions:    rowSessions{p, terminals, cfg.Turns},
		Background:  lc.Env,
		Wakes:       lc.Manager,
		Prompts:     &prompts.Env{DB: p.db, Home: home, Now: nowOf(cfg.Now)},
		NeedsYou:    p.needsYou(),
		SwitchOffer: p.switchOffer,
		Agents:      func() []string { return p.tickAgents(terminals) },
		Registered:  func() []string { return paths.ListAgents(home) },
		Config: func(agent string) (string, string, bool) {
			c, err := lc.roster.Read(agent)
			if err != nil {
				return "", "", false
			}
			_, model, effort := team.Parts(c)
			return model, effort, true
		},
		Account: func(agent string) (string, error) { return team.CurrentAccount(q, agent) },
		Parent: func(agent string) string {
			tm, err := team.LoadTeam(home)
			if err != nil || tm == nil {
				return ""
			}
			return tm.SupervisorOf(agent)
		},
		CacheLeft: func(agent string, now float64) (float64, bool, error) {
			acct, _ := team.CurrentAccount(q, agent)
			c := handoff.ReadClock(tu.ReadForAgent(agent, acct).TranscriptPath)
			if c == nil {
				return 0, false, nil
			}
			left, ok := c.Left(now)
			return left, ok, nil
		},
		CtxPct: func(agent, acct string) *int {
			st := tu.ReadForAgent(agent, acct)
			limit := tu.ContextLimit(agent, st.ModelID)
			if st.TrustedCtxUsed == nil || limit <= 0 {
				return nil
			}
			pct := *st.TrustedCtxUsed * 100 / limit
			return &pct
		},
		Now:      func() int64 { return nowOf(cfg.Now)().Unix() },
		Log:      func(s string) { cfg.Log("%s", s) },
		LoadHeld: lc.Gate.Held,
		Doing: func(agent string) (string, int64, bool) {
			if cfg.Turns == nil || !terminals {
				return "", 0, false
			}
			d := cfg.Turns.Doing(agent)
			if d == nil {
				return "", 0, false
			}
			return d.Text, d.At, true
		},
		Servers: cfg.Servers,
		LimitDialogOwned: func(agent string) bool {
			return p.limitDialogOwned(agent, tu, nowOf(cfg.Now)().Unix())
		},
		Jobs: func(agent string, now int64) []agentview.Job { return p.backgroundJobs(home, agent, now) },
		Walled: func(agent string, row *db.AgentState) *agentview.Wall {
			if yes, err := lc.Manager.WouldWake(row); err != nil || !yes {
				return nil
			}
			w := lc.Launch.WakeWait(q, agent, row)
			if w == nil {
				return nil
			}
			out := &agentview.Wall{Text: lifecycle.DescribeWakeWait(w), Slot: w.Slot, Pace: w.Pace != nil || w.FiveHour}
			switch {
			case w.Pace != nil:
				out.Reset = w.Pace.BackAt
			case w.FiveHour:
				out.Reset = w.Reset
			case !w.Slot:
				out.Reset = w.Reset
			}
			return out
		},
	}
}

// backgroundJobs is what runs behind agent's prompt, oldest first: its background shells while its footer reports work (the last as many
// as the footer counts), and its subagents still running.
func (p *Parts) backgroundJobs(home, agent string, now int64) []agentview.Job {
	var jobs []agentview.Job
	if has, err := p.LC.Env.HasBackgroundWork(p.db, agent); err == nil && has {
		count, _ := db.Cursor(p.db, "bgcount:"+agent)
		shells, _ := (&session.Env{DB: p.db, Home: home}).RunningJobs(agent, int(count))
		for _, j := range shells {
			jobs = append(jobs, agentview.Job{Kind: "shell", Label: j.Label(), Since: j.Started})
		}
	}
	subs, _ := db.RunningSubagents(p.db, agent, time.Unix(now, 0))
	for _, s := range subs {
		fg := !s.Background
		jobs = append(jobs, agentview.Job{Kind: "subagent", Label: s.Label(), Since: s.At, Foreground: &fg})
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].Since < jobs[j].Since })
	return jobs
}

// limitDialogOpen is whether the usage-limit dialog is on agent's screen.
func (p *Parts) limitDialogOpen(agent string) bool {
	if p.LC == nil || p.LC.term == nil {
		return false
	}
	handle, err := db.Terminal(p.db, agent)
	return err == nil && handle != "" && p.LC.term.UsageLimitOpen(handle)
}

// closeTriesBackS is how long after a close failed the limit dialog goes back to a person: the daemon cannot get past it just now.
const closeTriesBackS = 2 * states.AssumedAccountRecheckS // past the retry a failed close parks for, so it does not flicker

// limitHandled is whether the daemon is dealing with the limit dialog: the tick hands the agent off when its last turn ended on the limit
// or its reading fires a wall, and PerformHandoff does not wait for a turn behind that dialog. Anything else leaves it a person's.
func limitHandled(atLimit bool, t tick.Trigger, closeFailedAgoS int64, closeFailed bool) bool {
	if closeFailed && closeFailedAgoS < closeTriesBackS {
		return false
	}
	return atLimit || (t.Fired && (t.Kind == "usage" || t.Kind == "usage_7d"))
}

// limitDialogOwned is agentview.Env.LimitDialogOwned.
func (p *Parts) limitDialogOwned(agent string, tu tickUsage, now int64) bool {
	if !p.limitDialogOpen(agent) {
		return false
	}
	_, atLimit := tu.limitAt(p.db, agent)
	acct, _ := team.CurrentAccount(p.db, agent)
	st := tu.ReadForAgent(agent, acct)
	t := tick.Evaluate(st.ShortTrusted, st.TrustedCtxUsed, st.LongTrusted, tu.ContextLimit(agent, st.ModelID))
	failedAt, failed, err := db.LastEventTS(p.db, agent, "close_failed")
	return err == nil && limitHandled(atLimit, t, now-failedAt, failed)
}

// agentViewPass is the tick's pass that stores the views.
func agentViewPass(av *agentview.Env) tick.NamedSweep {
	return tick.NamedSweep{Name: "agent-view", Run: func(db.Q, int64) error { return av.Refresh() }}
}

// paneRuns is whether the pane agent's row names is up with something other than its shell in front (agentview.Env.PaneRuns).
func (p *Parts) paneRuns(agent string) bool {
	row, err := db.GetAgentState(p.db, agent)
	if err != nil || row == nil || row.Terminal.String == "" || p.LC == nil || p.LC.term == nil {
		return false
	}
	lister, ok := p.LC.term.Backend.(term.PaneLister)
	if !ok {
		return false
	}
	panes, ok := lister.ListPanes()
	if !ok {
		return false
	}
	for _, pane := range panes {
		if pane.Handle == row.Terminal.String {
			return !pane.Dead && !pane.AtShell()
		}
	}
	return false
}
