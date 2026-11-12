package compose

import (
	"errors"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/fold"
	"github.com/handline/handline/go/internal/sessionrpc"
	"github.com/handline/handline/go/scheduler/lifecycle"
	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/scheduler/pause"
)

// SessionRPC is the env of the session methods over the lifecycle: session.start, wake, stop, restart, park and interrupt.
func (p *Parts) SessionRPC(cfg Config) *sessionrpc.Env {
	c := controls{p: p, lc: p.LC}
	f := &fold.Env{Home: cfg.Home, DB: p.db, InTx: func(fn func(db.Q) error) error { return inTx(p.db, fn) }, Usage: p.LC.usage, Accounts: team.Accounts{Home: cfg.Home}, Today: today(cfg)}
	return &sessionrpc.Env{DB: p.db, Home: cfg.Home, Controls: c, Move: c.viaMover((*lifecycle.Mover).ForceMove), SwitchNow: c.viaMover((*lifecycle.Mover).SwitchNow), Now: cfg.Now, Fold: f.Fold, Roster: p.LC.roster,
		ForceStart: c.ForceStart, Unpass: c.Unpass, Launch: p.LC.prepareLaunch}
}

// prepareLaunch is session.launch_prepare: lifecycle.Launcher.Prepare for the terminal that asked.
func (lc *Lifecycle) prepareLaunch(r sessionrpc.LaunchRequest) (sessionrpc.LaunchResult, error) {
	var warnings []string
	site := &lifecycle.Site{Cwd: r.Cwd, Env: r.Env, Binary: r.Binary, HlExe: r.HlExe, Handle: r.Handle, WorktreeID: r.WorktreeID, PID: r.PID,
		Warn: func(s string) { warnings = append(warnings, s) }}
	got, err := lc.Launch.Prepare(site, r.Agent, r.Extra, r.Force, r.PromptFile, r.Account, r.Why, r.By)
	var refused *lifecycle.Refusal
	if errors.As(err, &refused) {
		return sessionrpc.LaunchResult{}, &sessionrpc.LaunchRefusal{Msg: refused.Msg}
	}
	if err != nil {
		return sessionrpc.LaunchResult{}, err
	}
	return sessionrpc.LaunchResult{Argv: got.Argv, Env: got.Env, Warnings: warnings}, nil
}

// ForceStart is agent.force_start: work the machine's load, its account's pace or its 5-hour limit holds for the agent goes past them for its next start,
// at the next tick. Refused when neither holds anything of the agent's: no work waits, or nothing holds it.
func (c controls) ForceStart(agent string) (bool, string, error) {
	m := c.lc.Manager
	why := c.lc.Gate.Held(agent)
	if pause.IsPaused(why) {
		return false, "New work is paused, and a force start does not pass it: resume new work, or send " + agent + " a message, which goes.", nil
	}
	row, err := db.GetAgentState(m.Q, agent)
	if err != nil {
		return false, "", err
	}
	owed, _, err := m.StartOrWakeNeeded(agent, row)
	if err != nil {
		return false, "", err
	}
	paced, pacedWhat := "", "its account is ahead of its weekly pace"
	if w := c.lc.Launch.WorkspaceWait(m.Q, agent); w != nil {
		return false, "A force start does not pass " + agent + "'s workspace rule: " + lifecycle.DescribeWakeWait(w) + ".", nil
	}
	if w := c.lc.Launch.WakeWait(m.Q, agent, row); w != nil && (w.Pace != nil || w.FiveHour) {
		paced = lifecycle.DescribeWakeWait(w)
		if w.FiveHour {
			pacedWhat = "Claude's 5-hour limit would run out before it starts over"
		}
	}
	if owed == "" && !c.lc.Launch.Ports.WaitingFor(m.Q, agent) {
		return false, "Nothing to start: " + agent + " has no work waiting.", nil
	}
	if why == "" && paced == "" {
		return false, "Nothing holds " + agent + " back: it starts by itself.", nil
	}
	var said []string
	for _, p := range []struct {
		held, kind, what string
		pass             func(string)
	}{{why, "load_limit_passed", loadgate.Cause(why), c.lc.Gate.Pass}, {paced, "pace_passed", pacedWhat, c.lc.Launch.PassPace}} {
		if p.held == "" {
			continue
		}
		p.pass(agent)
		if _, err := db.AddEvent(m.Q, agent, p.kind, db.KV{K: "held", V: p.held}); err != nil {
			return false, "", err
		}
		said = append(said, p.what)
	}
	return true, "Starting " + agent + " now, even though " + strings.Join(said, " and ") + ".", nil
}

// Unpass takes back what ForceStart let past, for a start that has not come by the time its caller stopped waiting.
func (c controls) Unpass(agent string) {
	c.lc.Gate.Unpass(agent)
	c.lc.Launch.UnpassPace(agent)
}

// controls is sessionrpc.Controls over the lifecycle manager.
type controls struct {
	p  *Parts
	lc *Lifecycle
}

var _ sessionrpc.Controls = controls{}

func (c controls) status(agent string) lifecycle.SessionStatus {
	m := c.lc.Manager
	return m.Status.ReadForAgent(agent, m.Agents.CurrentAccount(m.Q, agent))
}

// Start is control.start_result: a closed agent with a session to resume is woken; any other is spawned, in the worktree it has or a
// new one branched from base.
func (c controls) Start(call sessionrpc.Call, agent, repo, base string) (bool, string, error) {
	m := c.lc.Manager
	row, err := db.GetAgentState(m.Q, agent)
	if err != nil {
		return false, "", err
	}
	if m.ResumableWithoutTerminal(row) { // closed with a session to resume: resume it, whether or not anything waits
		ok, why, err := m.WakeAgent(agent, lifecycle.WakeOpts{Force: true, Person: call.Person})
		return ok, agent + ": " + why, err
	}
	if !m.Launch.ChooseAccount(m.Q, agent) {
		return false, fmt.Sprintf("%s: not started - %s", agent, m.Launch.NoRoom(agent)), nil
	}
	if repo == "" { // the agent's own repository, never the folder the daemon happens to run in
		repo = c.lc.roster.RepoOf(agent)
	}
	return c.lc.Spawns.SpawnWith(agent, repo, lifecycle.SpawnOpts{Base: base})
}

func (c controls) Wake(call sessionrpc.Call, agent string) (bool, string, error) {
	return c.lc.Manager.WakeAgent(agent, lifecycle.WakeOpts{Force: call.Person, Person: call.Person})
}

func (c controls) Close(call sessionrpc.Call, agent string, fresh bool) (bool, string, error) {
	st, o := c.status(agent), lifecycle.CloseOpts{Force: call.Person}
	if fresh {
		return c.lc.Manager.CloseFresh(agent, st, o)
	}
	return c.lc.Manager.CloseAgent(agent, st, o)
}

func (c controls) Restart(call sessionrpc.Call, agent string, fresh, force bool) (bool, string, error) {
	return c.lc.Manager.RestartAgent(agent, c.status(agent), lifecycle.RestartOpts{Fresh: fresh, Force: force, By: call.By, Cause: "restart"})
}

func (c controls) Compact(call sessionrpc.Call, agent string) (bool, string, error) {
	return c.lc.Manager.Compact(agent, c.status(agent), call.By)
}

// Park is control.park_result: a handoff, then parked until the window rolls over. The handoff is the policy tick's (perform_handoff), which
// this daemon does not run yet.
func (c controls) Park(_ sessionrpc.Call, agent string) (bool, string, error) {
	return false, agent + ": parking is the policy tick's handoff, which this daemon does not run yet", nil
}

// viaMover is a call of the lifecycle's mover: handoff.force_move for agent.move, the answer to a switch_offer for agent.switch_now.
func (c controls) viaMover(do func(m *lifecycle.Mover, agent, account, by string) (bool, string, error)) func(sessionrpc.Call, string, string) (bool, string, error) {
	return func(call sessionrpc.Call, agent, account string) (bool, string, error) {
		if c.lc.Mover == nil {
			return false, agent + ": the move is the handoff's, which this daemon has not built", nil
		}
		return do(c.lc.Mover, agent, account, call.By)
	}
}

// Context is the model id and context figures of an agent on an account (agent.strip). The percent is the status file's own, nil when
// stale, as the usage read gives it; the tokens and the limit are the ones the policy tick judges the agent by. Before the lifecycle is
// built there are none.
func (p *Parts) Context(agent, account string) (string, *int, *int64, int) {
	if p.LC == nil {
		return "", nil, nil, 0
	}
	tu := tickUsage{p: p, u: p.LC.usage, roster: p.LC.roster}
	st := tu.ReadForAgent(agent, account)
	var used *int64
	if st.TrustedCtxUsed != nil {
		n := int64(*st.TrustedCtxUsed)
		used = &n
	}
	return st.ModelID, p.LC.usage.ReadForAgent(agent, account).TrustedCtxPct(), used, tu.ContextLimit(agent, st.ModelID)
}
