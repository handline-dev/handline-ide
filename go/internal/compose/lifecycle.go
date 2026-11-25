package compose

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/accounts/usagepoll"
	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/peerinbox"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/claudetrust"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/lock"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/sessiontoken"
	"github.com/handline/handline/go/scheduler/lifecycle"
	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/scheduler/pause"
	"github.com/handline/handline/go/scheduler/settings"
	"github.com/handline/handline/go/scheduler/worktreelock"
	"github.com/handline/handline/go/scheduler/worktreepool"
	"github.com/handline/handline/go/taskboard/handoff"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// This file builds the lifecycle manager over the structured sessions and the terminal host (termport.go). A terminal agent is launched,
// ended and woken through the host; what is typed at a running terminal session is not served here (messages go in through its inbox).

// Lifecycle is the lifecycle manager and what is built beside it.
type Lifecycle struct {
	Manager *lifecycle.Manager
	Spawns  *lifecycle.Spawns
	Launch  *lifecycle.Launcher
	Handoff *handoff.Env
	Env     *supervisor.Env
	// Mover is the handoff and the move to another account.
	Mover *lifecycle.Mover
	// Desks is the worktree pool a start or a wake takes a desk from.
	Desks *worktreepool.Pool

	// Gate holds the daemon's own starts while the machine is over a load limit (agent.force_start passes it).
	Gate *loadgate.Gate

	usage  *usage.Env
	roster *team.Roster
	term   *term.Term
}

// LifecycleConfig is what only the daemon main knows beyond Config.
type LifecycleConfig struct {
	// Env is the telling layer; it is the Delivery the lifecycle reads and the Heads the handoff record reads.
	Env *supervisor.Env
}

// Lifecycle builds the manager over p's runner. The database must be bound.
func (p *Parts) Lifecycle(cfg Config, lc LifecycleConfig) *Lifecycle {
	q, home := p.db, cfg.Home
	roster := team.NewRoster(home)
	uenv := &usage.Env{Home: home, Now: func() float64 { return float64(nowOf(cfg.Now)().UnixNano()) / 1e9 }}
	accounts := team.Accounts{Home: home}
	routing := func() *team.Team { t, _ := team.LoadTeam(home); return t }
	keptRules := &team.KeptBlocks{Q: q}
	kept, errs := team.MigratePeerRules(home)
	for _, f := range kept {
		cfg.Log("  team rules: %s keeps the old reports wording its team changed", f)
	}
	for _, err := range errs {
		cfg.Log("  team rules: not moved to the peers wording: %v", err)
	}
	moved, errs := team.ImportInstructions(home, q) // after MigratePeerRules, which still rewrites the files
	if len(moved) > 0 {
		cfg.Log("  instructions: %d files moved into the database", len(moved))
	}
	for _, err := range errs {
		cfg.Log("  instructions: a file not moved into the database: %v", err)
	}
	var founderTyped func(agent string) bool // the founder's words typed at the agents the peer inbox reaches
	if cfg.FounderTyped {
		founderTyped = p.Peer
	}
	if lc.Env != nil {
		lc.Env.FounderTyped = founderTyped
		lc.Env.ShowsTyped = peerinbox.ShowsTyped
		if founderTyped != nil {
			lc.Env.Typeable = p.typeable
		}
	}
	launcher := &lifecycle.Launcher{Q: q, Home: home, Roster: roster, Tokens: sessiontoken.New(home), Now: cfg.Now, FounderTyped: founderTyped, Ports: lifecycle.LaunchPorts{
		Usage:          &lifecycle.UsageAdapter{Env: uenv, Q: q},
		Accounts:       accounts,
		Vendors:        launchVendors{vendors{home: home, roster: roster}},
		CurrentAccount: func(q db.Q, agent string) string { a, _ := team.CurrentAccount(q, agent); return a },
		// The reading is made on the database, not inside a call's transaction: the lifecycle is never called inside one.
		AccountReading: func(q db.Q, agent string) (string, string, bool) {
			r, ok, err := team.ReadAccount(q.(*sql.DB), agent, nil)
			if err != nil || !ok {
				return "", "", false
			}
			return r.Account, r.Source, true
		},
		// The ports a terminal's own `hl launch` has too (cmd/hl/launch.go): a nil one panics in Build and in FingerprintParts.
		WaitingFor: func(q db.Q, agent string) bool {
			head, err := db.EffectiveHead(q, agent)
			return err != nil || head != nil // a read that fails says something may be waiting
		},
		TeamRules: func(agent string) []string {
			t := routing()
			if err := team.SeedRules(q, t, nowOf(cfg.Now)().Unix()); err != nil {
				cfg.Log("  team rules: not seeded: %v", err)
			}
			return team.Rules(q, agent, t, keptRules.Block)
		},
		Held: func(agent string) team.Held {
			t := routing()
			if t == nil {
				return team.Held{}
			}
			id, ok, err := team.WorkspaceID(q, t.TeamOf(agent))
			if err != nil || !ok {
				if err != nil {
					cfg.Log("  kept rules: not read for %s: %v", agent, err)
				}
				return team.Held{}
			}
			h, err := team.HeldFor(q, id)
			if err != nil {
				cfg.Log("  kept rules: not read for %s: %v", agent, err)
			}
			return h
		},
		Routing:     routing,
		McpApproved: team.NewTrust(home).MCPApproved,
	}}
	launchPort := lifecycle.LaunchPort{L: launcher}
	ho := &handoff.Env{DB: q, Home: home, Heads: lc.Env, Now: cfg.Now,
		AgentSession: func(agent, account string) (string, string) {
			st := uenv.ReadForAgent(agent, account)
			return st.SessionID, st.Transcript
		}}
	settings := &settings.Settings{Home: home, Roster: roster}
	ho.Note = handoff.NoteEnv{Settings: noteSettings{settings, roster}, Telling: lc.Env, Log: cfg.Log}
	if cfg.CLI != "" {
		meter := &usagepoll.Meter{Home: home, Claude: cfg.CLI, Env: cfg.Env, Now: cfg.Now}
		ho.Summary = &handoff.Summarizer{Log: cfg.Log, Run: summaryRun(home, launcher.MayStartHere, meter, uenv, q)}
		p.summary = ho.Summary
	}
	tm := cfg.Term
	if tm == nil {
		tm = newTerm(home, q, cfg.Nodes)
	}
	if cfg.Turns != nil {
		tm.Turns = cfg.Turns
	}
	var onLaunch func(string) // the view is told through the turns tracker, which the tick hands it
	if cfg.Turns != nil {
		onLaunch = cfg.Turns.Launched
	}
	spawns := &lifecycle.Spawns{Q: q, Host: termHost{tm}, Launch: launchPort, Delivery: lc.Env, Roster: roster, Now: cfg.Now,
		Locks: p.Locks, OnLaunch: onLaunch, TrustFolder: trustFolder(home, vendors{home: home, roster: roster}, cfg.Log)}
	// A launch a terminal's own `hl launch` asks for is built and recorded here (session.launch_prepare), so it joins the team's Run and
	// is refused from a pane stamped for another agent the way the daemon's own starts are.
	launcher.Ports.RegisterWorker = func(agent, handle string) error { _, err := spawns.RegisterWorker(agent, handle); return err }
	launcher.Ports.PaneAgent = func(handle string) string {
		if h, ok := agenthost.Tmux(home).(term.PaneAgenter); ok {
			a, _ := h.PaneAgent(handle)
			return a
		}
		return ""
	}
	waiting := func(q db.Q, agent string) bool {
		head, err := lc.Env.EffectiveHead(q, agent)
		if err != nil || head != nil {
			return true // a read that fails says something may be waiting: the wake is cheap to refuse and dear to miss
		}
		return false
	}
	launcher.Ports.WaitingFor = waiting
	gate := &loadgate.Gate{Limits: func() loadgate.Limits { return loadgate.LimitsOf(usage.LoadConfig(home)) },
		// A read that fails counts none mid-turn, which holds nothing: the gate must never be what stalls work.
		Active: func() int {
			by, _ := uenv.ActiveByAccount(q)
			n := 0
			for _, agents := range by {
				n += len(agents)
			}
			return n
		}, Machine: cfg.Machine, Now: cfg.Now,
		Paused: func() string { return pause.Why(q, nowOf(cfg.Now)()) }}
	// The start and delivery paths hold work with the ceiling on it; the agent view shows the hold as it stands (gate.Held).
	forWork := func(agent string) string {
		return gate.ForWork(agent, func(why string) {
			if _, err := db.AddEvent(q, agent, "load_limit_passed", db.KV{K: "held", V: why}, db.KV{K: "by", V: "ceiling"}); err != nil {
				cfg.Log("%s: load ceiling pass not recorded: %v", agent, err)
			}
			cfg.Log("  %s: held %s by the load, now let go once - %s", agent, loadgate.Ceiling, why)
		})
	}
	lc.Env.LoadHeld = forWork
	lc.Env.Paused = gate.Paused
	lc.Env.ActNow = p.ActNow
	desks := &worktreepool.Pool{DB: q, Setup: spawns.Prepare, Now: cfg.Now,
		Running: func(repo, desk string) bool { return worktreelock.Held(repo, desk, worktreelock.Running) != "" },
		Base: func(repo, branch string) (string, string, error) {
			if branch == "" {
				branch = gitrun.DefaultBranch(repo)
			}
			return lifecycle.PickBase(repo, branch)
		},
		Branch: func(agent string) string { return paths.AgentHandle(home, agent) }}
	spawns.Desks = desks
	hp := &handoffPort{env: ho, q: q, now: nowOf(cfg.Now)}
	m := &lifecycle.Manager{Q: q, Home: home, Log: cfg.Log, Settings: settings,
		Delivery: lc.Env, Handoff: hp, Term: termPort{tm}, Blocked: noBlocked{}, Launch: launchPort, Spawn: spawns,
		Agents: agentsPort{home: home, roster: roster, accounts: accounts, env: lc.Env, usage: uenv}, Status: &lifecycle.StatusAdapter{Env: uenv, Q: q},
		Observe: observer{p: p}, Typing: terminalLock{home: p.home}, TermRun: termRun{p}, Desks: desks, Locks: p.Locks, OnLaunch: onLaunch,
		LoadHeld: forWork, OnStartStall: tellOpenerOfStall(lc.Env, cfg.Log), WhatChanged: launcher.WhatChanged,
		Now: func() int64 { return nowOf(cfg.Now)().Unix() }}
	p.LC = &Lifecycle{Manager: m, Spawns: spawns, Desks: desks, Launch: launcher, Handoff: ho, Env: lc.Env, Gate: gate, usage: uenv, roster: roster, term: tm}
	hp.limit = tickUsage{roster: roster}.ContextLimit
	hp.mover = func() *lifecycle.Mover { return p.LC.Mover }
	m.RefreshView = func(agent string) {
		if v := p.view.Load(); v != nil {
			v.RefreshOne(agent)
		}
	}
	p.LC.Mover = p.newMover(m, uenv, lc.Env)
	if cfg.Nodes != nil { // an agent on a runner is launched from here, on an account logged in there, and its note brought here before it is read
		cfg.Nodes.Launch = p.LC.runnerLaunch
		p.LC.Launch.Ports.Devices = cfg.Nodes
		ho.Fetch = cfg.Nodes.Flush
	}
	return p.LC
}

// tellOpener tells a notice about agent to the agent that opened the oldest task waiting for it, else the founder (db.AgentNoticeTo): the
// daemon log alone reaches no one.
func tellOpener(env *supervisor.Env, log func(string, ...any), q db.Q, agent, text, reason string) {
	to, tid, err := noticeRoute(q, agent)
	if err == nil {
		err = env.TellAbout(q, to, agent, tid, text, reason, time.Now().Unix())
	}
	if err != nil {
		log("  %s: %s was not told to %s: %v", agent, reason, to, err)
	}
}

// noticeRoute is who hears that agent went wrong, and about which task.
func noticeRoute(q db.Q, agent string) (string, int64, error) {
	to, tid, err := db.AgentNoticeTo(q, agent)
	if actor.IsPerson(to) {
		tid = 0 // the founder's warning is about the agent: it leaves when the agent is mended, which the task's opener cannot do
	}
	return to, tid, err
}

// tellOpenerOfStall tells that agent's starts for waiting work failed and it is stalled.
func tellOpenerOfStall(env *supervisor.Env, log func(string, ...any)) func(db.Q, string, string) {
	return func(q db.Q, agent, why string) {
		tellOpener(env, log, q, agent, fmt.Sprintf("%s is STALLED: %d starts for the work waiting for it failed. The last said: %s. "+
			"Fix that (often its workspace folder), then clear the stall in the app.", addr.Ref(agent), states.MaxFailedAttempts, why), "start_stall")
	}
}

// badRepo is the agents whose one repo broke, all told to the same one about the same task.
type badRepo struct {
	repo, why, to string
	tid           int64
	agents        []string
}

// CheckRepos tells when the repo an agent's row names is no git checkout, since a start there fails. Once per break: a daemon restart does
// not tell it again, and a repo fixed and broken again is told again. Agents that share the broken repo (a workspace's folder gone) and
// the one who hears it are told in one notice, named, not one per agent. The founder's notice is kept under its first agent and leaves
// when that one is mended, so a mend on a repo tells the founder again of the agents still broken on it.
func CheckRepos(q db.Q, home string, env *supervisor.Env, log func(string, ...any)) {
	type seen struct{ agent, repo, why, told string }
	roster := team.NewRoster(home)
	var all []seen
	mended := map[string]bool{} // repos an agent was told of and is off now
	for _, agent := range paths.ListAgents(home) {
		repo, err := roster.ReadRepo(agent)
		if err != nil {
			continue // not known is no mend: an unread roster must not end a warning
		}
		why := ""
		if repo != "" {
			why = team.CheckoutProblem(repo)
		}
		_, last, _, err := db.LastEvent(q, agent, "bad_repo")
		if err != nil {
			continue
		}
		told, _ := last["repo"].(string)
		all = append(all, seen{agent, repo, why, told})
		if why == "" && told == "" || why != "" && told == repo {
			continue
		}
		bad := repo
		if why == "" {
			bad = "" // fixed: the record says nothing is told
		}
		if _, err := db.AddEvent(q, agent, "bad_repo", db.KV{K: "repo", V: bad}); err != nil {
			log("  %s: bad_repo not recorded: %v", agent, err)
			all = all[:len(all)-1]
			continue
		}
		if told != "" {
			mended[told] = true
		}
	}
	var groups []*badRepo
	for _, a := range all {
		if a.why == "" {
			continue
		}
		again := a.told == a.repo
		if again && !mended[a.repo] {
			continue
		}
		to, tid, err := noticeRoute(q, a.agent)
		if err != nil {
			log("  %s: bad_repo was not told: %v", a.agent, err)
			continue
		}
		if again && !actor.IsPerson(to) {
			continue // an agent's notice of it is its own; only the founder's shared one left with the mended agent
		}
		if !again {
			log("  %s: repo %s %s", a.agent, a.repo, a.why)
		}
		groups = addBadRepo(groups, badRepo{repo: a.repo, why: a.why, to: to, tid: tid}, a.agent)
	}
	for _, g := range groups {
		if err := env.TellAbout(q, g.to, g.agents[0], g.tid, badRepoText(g), "bad_repo", time.Now().Unix()); err != nil {
			log("  %s: bad_repo was not told to %s: %v", g.agents[0], g.to, err)
		}
	}
}

// RecheckRepos is CheckRepos again when an agent told of a broken repo has a repo that is no longer that broken one: its folder came back,
// or the agent was given another. It looks only at the agents told, so a tick pays a few stats, and a fixed folder clears its notice
// without a daemon restart.
func RecheckRepos(q db.Q, home string, env *supervisor.Env, log func(string, ...any)) error {
	rows, err := q.Query("SELECT agent, payload, MAX(id) FROM events WHERE kind = 'bad_repo' GROUP BY agent")
	if err != nil {
		return err
	}
	told := map[string]string{}
	for rows.Next() {
		var agent string
		var payload sql.NullString
		var id int64
		if err := rows.Scan(&agent, &payload, &id); err != nil {
			rows.Close()
			return err
		}
		var p struct{ Repo string }
		if json.Unmarshal([]byte(payload.String), &p) == nil && p.Repo != "" {
			told[agent] = p.Repo
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(told) == 0 {
		return err
	}
	roster := team.NewRoster(home)
	for _, agent := range paths.ListAgents(home) { // CheckRepos looks only at these: one gone keeps its last record
		repo, was := told[agent]
		if !was {
			continue
		}
		now, err := roster.ReadRepo(agent)
		if err != nil {
			return err
		}
		if now != repo || team.CheckoutProblem(now) == "" {
			CheckRepos(q, home, env, log)
			return nil
		}
	}
	return nil
}

// addBadRepo puts agent in the group of b's repo and receiver, opening one when none is.
func addBadRepo(groups []*badRepo, b badRepo, agent string) []*badRepo {
	for _, g := range groups {
		if g.repo == b.repo && g.why == b.why && g.to == b.to && g.tid == b.tid {
			g.agents = append(g.agents, agent)
			return groups
		}
	}
	b.agents = []string{agent}
	return append(groups, &b)
}

// badRepoNamed is how many agents a bad-repo notice names before it counts the rest.
const badRepoNamed = 3

// badRepoText says which agents cannot start and why, each agent as a reference its reader sees by name and the repo under ~.
func badRepoText(g *badRepo) string {
	refs := make([]string, 0, badRepoNamed)
	for _, a := range g.agents[:min(len(g.agents), badRepoNamed)] {
		refs = append(refs, addr.Ref(a))
	}
	who := refs[0]
	switch rest := len(g.agents) - len(refs); {
	case rest > 0:
		who = fmt.Sprintf("%s and %d more", strings.Join(refs, ", "), rest)
	case len(refs) > 1:
		who = strings.Join(refs[:len(refs)-1], ", ") + " and " + refs[len(refs)-1]
	}
	if len(g.agents) == 1 {
		return fmt.Sprintf("%s cannot be started: its repo %s %s. Give it the absolute path of a git checkout (its workspace folder).",
			who, homeShort(g.repo), g.why)
	}
	return fmt.Sprintf("%s cannot be started: their repo %s %s. Give them the absolute path of a git checkout (their workspace folder).",
		who, homeShort(g.repo), g.why)
}

// homeShort is path with the user's home written as ~.
func homeShort(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}

func nowOf(f func() time.Time) func() time.Time {
	if f != nil {
		return f
	}
	return time.Now
}

// vendors is lifecycle.VendorsPort: this daemon launches no vendor's command line.
type vendors struct {
	home   string
	roster *team.Roster
}

func (v vendors) ForAgent(agent string) (cli.Agent, error) {
	return nil, fmt.Errorf("%s: a vendor driver launches a terminal agent, which this daemon does not serve", agent)
}

// ForAgent is the Agent of the vendor agent runs, by the vendor's name.
func (v launchVendors) ForAgent(agent string) (cli.Agent, error) {
	vendor := v.AgentVendor(agent)
	drv, ok := cli.New(vendor, agenthost.Host(v.home))
	if !ok {
		return nil, fmt.Errorf("vendor %q has no launchable agent", vendor)
	}
	return drv, nil
}

// launchVendors is vendors with each vendor's Agent, for the launches a terminal's `hl launch` asks this daemon to build.
type launchVendors struct{ vendors }

func (v vendors) AgentVendor(agent string) string {
	cfg, err := v.roster.Read(agent)
	if err != nil {
		return team.DefaultVendor
	}
	vendor, _, _ := team.Parts(cfg)
	return vendor
}

func (v vendors) AccountVendor(acct string) string { return team.AccountVendor(v.home, acct) }

func (v vendors) ConfigDir(acct string) string {
	dir, err := team.AccountConfigDir(v.home, acct)
	if err != nil {
		return ""
	}
	return dir
}

var _ lifecycle.VendorsPort = vendors{}

// trustFolder marks folder trusted in every claude account's config, so the session opens without claude's folder-trust dialog on
// whichever account it starts or later moves to, and turns off claude's question whether to resume a long-idle conversation, which
// Handline decides itself, the first-run onboarding a freshly logged-in account would stop the launch on, and auto mode's one-time
// question before a read outside the working folders. Only a write that happened or failed is logged; a launch never waits on it.
func trustFolder(home string, v vendors, log func(string, ...any)) func(agent, folder string) {
	if log == nil {
		log = func(string, ...any) {}
	}
	return func(agent, folder string) {
		if v.AgentVendor(agent) != team.DefaultVendor {
			return
		}
		keys := claudetrust.Keys(folder)
		for _, acct := range paths.ListAccounts(home) {
			dir := ""
			if v.AccountVendor(acct) == team.DefaultVendor {
				dir = v.ConfigDir(acct)
			}
			if dir == "" {
				continue
			}
			switch out, err := claudetrust.Grant(claudetrust.ConfigFile(dir), keys); {
			case err != nil:
				log("  %s: could not mark %s trusted for account %s: %v", agent, folder, acct, err)
			case out == claudetrust.Granted:
				log("  %s: marked %s trusted for account %s", agent, folder, acct)
			case out == claudetrust.Locked || out == claudetrust.Unreadable:
				log("  %s: did not mark %s trusted for account %s (%s config)", agent, folder, acct, out)
			}
			switch out, err := claudetrust.Dismiss(claudetrust.ConfigFile(dir)); {
			case err != nil:
				log("  %s: could not turn off the resume, first-run and outside-read questions for account %s: %v", agent, acct, err)
			case out == claudetrust.Granted:
				log("  %s: turned off the resume, first-run and outside-read questions for account %s", agent, acct)
			}
		}
	}
}

// agentsPort is lifecycle.AgentsPort over the roster and the accounts.
type agentsPort struct {
	home     string
	roster   *team.Roster
	accounts team.Accounts
	env      *supervisor.Env
	usage    *usage.Env
}

var _ lifecycle.AgentsPort = agentsPort{}

func (a agentsPort) RepoOf(agent string) string { return a.roster.RepoOf(agent) }

func (a agentsPort) ReadConfig(agent string) error {
	_, err := a.roster.Read(agent)
	return err
}

func (a agentsPort) CurrentAccount(q db.Q, agent string) string {
	acct, _ := team.CurrentAccount(q, agent)
	return acct
}

// MeasureBegin starts the cold-start measurement of a move: what the new account's usage was when the agent was put on it.
func (a agentsPort) MeasureBegin(q db.Q, agent, account string) {
	if err := a.usage.BeginColdStart(q, agent, account); err != nil {
		if a.env.Log != nil {
			a.env.Log("  %s: the cold-start measurement did not begin: %v", agent, err)
		}
	}
}

// RaiseDecision puts a decision in front of the founder about an agent: a task of theirs with the request as its card.
func (a agentsPort) RaiseDecision(q db.Q, agent, text, reason string) error {
	_, err := a.env.RaiseDecision(q, agent, text, reason, "", "", 0, 0)
	return err
}

func (a agentsPort) VendorName(agent string) string {
	cfg, err := a.roster.Read(agent)
	if err != nil {
		return team.DefaultVendor
	}
	vendor, _, _ := team.Parts(cfg)
	return vendor
}

// handoffPort is lifecycle.HandoffPort over the handoff record.
type handoffPort struct {
	env   *handoff.Env
	q     db.Q
	now   func() time.Time
	mover func() *lifecycle.Mover
	limit func(agent, modelID string) int // the agent's context limit
}

var _ lifecycle.HandoffPort = (*handoffPort)(nil)

func (h *handoffPort) Capture(_ db.Q, agent string) (string, []string, error) {
	return h.env.Capture(agent, "")
}

func (h *handoffPort) Record(_ db.Q, agent, closedSession string) ([]string, error) {
	return h.env.Record(agent, closedSession)
}

func (h *handoffPort) NoteFromTranscript(agent string) bool { return h.env.NoteFromTranscript(agent) }

func (h *handoffPort) MemoryLine(agent string) string {
	if h.env.MemoryLine == nil {
		return ""
	}
	return h.env.MemoryLine(agent)
}

func (h *handoffPort) PreviousSession(_ db.Q, agent, account string) (string, string) {
	s, path, err := h.env.PreviousSession(agent, account, "")
	if err != nil {
		return "", ""
	}
	return s, path
}

func (h *handoffPort) RequestLine(_ db.Q, agent string) string {
	line, err := h.env.RequestLine(agent)
	if err != nil {
		return ""
	}
	return line
}

// MovePlan is the account a walled agent can move to: the same plan the tick makes, so a wake and a handoff agree.
func (h *handoffPort) MovePlan(_ db.Q, agent, kind string, starting bool) (string, string) {
	dest, why, err := h.mover().MovePlan(agent, kind, starting)
	if err != nil {
		return "", err.Error()
	}
	return dest, why
}

// BackingOff is daemon/handoff._handoff_backing_off: a handoff that keeps degrading is not retried every tick.
func (h *handoffPort) BackingOff(q db.Q, agent string) bool {
	row, err := db.GetAgentState(q, agent)
	if err != nil || row == nil || row.ThrottleCount == 0 {
		return false
	}
	var last sql.NullInt64
	if q.QueryRow("SELECT MAX(updated) FROM handoff WHERE agent = ?", agent).Scan(&last) != nil || !last.Valid {
		return false
	}
	return h.now().Unix() < last.Int64+int64(states.BackoffFor(int(row.ThrottleCount)).Seconds())
}

func (h *handoffPort) AnsweredTurn(_ db.Q, agent string) bool {
	ok, err := h.env.AnsweredTurn(agent)
	return err == nil && ok
}

func (h *handoffPort) CacheClock(_ db.Q, agent, session string) (int64, int64, bool, bool) {
	c, err := h.env.CacheClock(agent, session)
	if err != nil || c == nil {
		return 0, 0, false, false
	}
	return int64(c.At), int64(c.TTL), c.TTL != 0, true
}

func (h *handoffPort) CacheMargin(ttl int64) int64 { return int64(handoff.Margin(int(ttl))) }

func (h *handoffPort) ContextUsed(_ db.Q, _, session string) (int64, bool) {
	used, _, ok := lastContext(h.env.FindTranscript(session, ""))
	return used, ok
}

func (h *handoffPort) ContextFull(_ db.Q, agent, session string) bool {
	used, model, ok := lastContext(h.env.FindTranscript(session, ""))
	if !ok || h.limit == nil {
		return false
	}
	limit := h.limit(agent, model)
	return limit > 0 && used >= int64(limit)
}

func (h *handoffPort) ClearNote(_ db.Q, agent string) { _ = h.env.ClearNote(agent) }

// observer is the process table of the terminal agents' sessions, with each one's pid.
type observer struct{ p *Parts }

var _ lifecycle.Observer = observer{}

func (o observer) LiveSessions() (map[string]int, bool) {
	return o.p.terminalSessions(), true
}

// terminalLock is the right to type at a terminal agent, the lock every typing call takes.
type terminalLock struct{ home string }

func (t terminalLock) TakeTerminal(agent string) (func(), bool) {
	h := lock.TakeTerminal(t.home, agent)
	if h == nil {
		return nil, false
	}
	return h.Release, true
}

const notServed = "terminal agents are not served by this daemon"

// noBlocked: this daemon gets past no dialogs.
type noBlocked struct{}

var _ lifecycle.Blocked = noBlocked{}

func (noBlocked) NoteCloseFailure(db.Q, string)          {}
func (noBlocked) PastDialog(db.Q, string) (bool, string) { return false, notServed }
func (noBlocked) StopWorkAndExit(db.Q, string) bool      { return false }

// noteSettings is handoff.Settings: config.yaml's flags, and an agent's own boolean column.
type noteSettings struct {
	*settings.Settings
	roster *team.Roster
}

func (n noteSettings) AgentFlag(agent, key string) (value, ok bool) {
	cfg, err := n.roster.Read(agent)
	if err != nil {
		return false, false
	}
	value, ok = cfg[key].(bool)
	return value, ok
}
