package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/scheduler/access"
)

// Site is where a launch runs, as the caller's process knows it: the daemon builds the launch for a terminal that is not its own, so
// nothing about the caller is read from the daemon's process.
type Site struct {
	Cwd string
	// Env is the caller's environment, which the session's starts from.
	Env map[string]string
	// Binary is the vendor's binary found on the caller's PATH; "" asks the driver.
	Binary string
	// HlExe is the hl the vendor's hooks call.
	HlExe string
	// Handle and WorktreeID are the terminal the caller runs in ("" for none).
	Handle, WorktreeID string
	// PID is the caller's process, which becomes the session.
	PID int
	// Warn is told what the person at the terminal should read; nil drops it.
	Warn func(string)
}

func (s *Site) warn(format string, a ...any) {
	if s.Warn != nil {
		s.Warn(fmt.Sprintf(format, a...))
	}
}

// Build is the argv and environment ready to hand to execve for agent. extra are the caller's own arguments, force overrides the usage
// wall, account is a destination someone already chose. A launch that must not go ahead is a *Refusal.
func (l *Launcher) Build(site *Site, agent string, extra []string, force bool, account string) ([]string, map[string]string, error) {
	p := l.Ports
	if l.Tokens == nil {
		// Without the key store nothing would be hidden from the session and no proof or key recorded for it: a wiring that forgot it must not fail open.
		return nil, nil, errors.New("launch has no session token store")
	}
	l.Tokens.EnsureKeyFolders()
	// Its folder is by key: one not looked up would read ROLE.md and memory from a folder sessions may write.
	if key, err := paths.AgentKeyIn(l.Q, agent); err != nil {
		return nil, nil, err
	} else if key == "" {
		return nil, nil, refuse("agent '%s' is not registered: no agent row has it", agent)
	}
	drv, err := p.Vendors.ForAgent(agent)
	if err != nil {
		return nil, nil, refuse("agent '%s': %v", agent, err)
	}
	resuming := drv.IsResume(extra)
	acct := l.ChooseAccount(agent, resuming, account)
	if acct == "" {
		if resuming {
			return nil, nil, refuse("agent '%s' has no recorded session to resume", agent)
		}
		return nil, nil, refuse("agent '%s': %s", agent, l.NoRoom(agent))
	}
	// Its config dir is by key: one not looked up would point the session at an empty folder that is no account.
	if key, err := paths.AccountKeyIn(l.Q, acct); err != nil {
		return nil, nil, fmt.Errorf("account %s: %w", acct, err)
	} else if key == "" {
		return nil, nil, refuse("account '%s' is not registered: no account row has it", acct)
	}
	// Without its config dir the session would run on whatever account the environment names, the user's own at worst.
	if _, err := team.AccountConfigDir(l.Home, acct); err != nil {
		return nil, nil, refuse("account '%s': %v", acct, err)
	}
	// The last line before an exec: whatever chose this account, a session is this vendor's binary pointed at that account's credentials.
	if theirs := p.Vendors.AccountVendor(acct); theirs != drv.Name() {
		return nil, nil, refuse("agent '%s' is a %s agent and account '%s' is a %s account - refusing to start it there.", agent, drv.Name(), acct, theirs)
	}
	if resuming && drv.Name() == "claude" {
		if err := carrySession(l.Home, acct, extra); err != nil {
			site.warn("warning: the session %s resumes is under another account and could not be copied to %s: %v", resumeID(extra), acct, err)
		}
	}

	if hooks := drv.Hooks(); hooks != nil {
		// An unreadable settings file is doctor's to report, not a reason not to start.
		if hooks.EnsureShared() == nil {
			// A wanted event added after the shared settings were last installed would otherwise sit uninstalled for good: every launch is
			// where install gets the chance to catch up.
			if hooks.Missing() {
				if err := hooks.Install(site.HlExe); err != nil {
					site.warn("warning: Handline's own hooks are behind and could not be installed (%v) - run `hl init` from outside an agent's sandbox", err)
				}
			}
			if err := hooks.AllowHomeSocket(); err != nil {
				site.warn("warning: agents' sandbox cannot reach Handline's socket and it could not be allowed (%v) - run `hl init` from outside an agent's sandbox", err)
			}
		}
	}
	if !p.Accounts.MayStart(acct) {
		login, _ := p.Accounts.LoginCommand(acct)
		return nil, nil, refuse("account '%s' is not logged in.\nA human has to do this once (Handline never touches credentials):\n\n  %s\n", acct, login)
	}

	// Only a current reading may block a launch. Neither a missing nor a stale snapshot can: the session about to start is the only thing
	// that writes one. Staleness still gates assignment, in the usage port.
	st := p.Usage.Read(acct)
	if st.Exists && st.Stale {
		age := "unknown age"
		if st.AgeS != nil {
			age = fmt.Sprintf("%ds", int(*st.AgeS))
		}
		site.warn("warning: account '%s' usage is %s old and may be out of date; the daemon will ask the endpoint about it on its next tick.", acct, age)
	} else if !force && st.Exists && st.ShortTrusted != nil && *st.ShortTrusted >= p.Usage.ShortBlock() {
		reset := "None"
		if st.ShortReset != 0 {
			reset = fmt.Sprint(st.ShortReset)
		}
		return nil, nil, refuse("account '%s' is at %d%% of the %s limit (>= %d%%) - refusing to start.\nresets at %s; use --force to override",
			acct, *st.ShortTrusted, st.ShortLabel, p.Usage.ShortBlock(), reset)
	}

	exe := site.Binary
	if exe == "" {
		exe = drv.Binary()
	}
	if exe == "" {
		return nil, nil, refuse("cannot find %s on PATH", drv.Name())
	}

	cfg, err := l.Roster.Read(agent)
	if err != nil {
		return nil, nil, err
	}
	// Always pass effort explicitly: left unset it comes from whatever the environment carries.
	effort := l.effortOf(cfg)
	if !slices.Contains(team.ValidEffort, effort) {
		return nil, nil, refuse("agent '%s' has effort=%s; must be one of %s", agent, pyRepr(effort), strings.Join(team.ValidEffort, ", "))
	}

	mcpArg, mcpErr := l.mcpConfigArg()
	if mcpErr != "" {
		site.warn("warning: shared mcp config %s, skipping --mcp-config", mcpErr)
	}

	preamble, err := l.ContextFor(agent)
	if err != nil {
		return nil, nil, err
	}
	memoryDir := filepath.Join(paths.AgentDir(l.Home, agent), "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		return nil, nil, err
	}
	// A vendor with no auto mode never reads `trusted`.
	chosen := l.Roster.ModeOf(agent, cfg)
	var mode []string
	if chosen == team.DefaultMode {
		mode = drv.AutoModeArgv()
	} else {
		mode = drv.PermissionModeArgv(chosen)
	}
	if chosen != team.DefaultMode && len(mode) == 0 {
		site.warn("warning: agent '%s' has mode %s, but %s has no such mode - starting it as usual", agent, chosen, drv.Name())
	}
	cwd := site.Cwd
	var approved []string
	if p.McpApproved != nil {
		approved = p.McpApproved(cwd)
	}
	extraDirs, err := l.ExtraWorktreesOf(agent, cfg)
	if err != nil {
		return nil, nil, err
	}
	hide, noWrite, err := l.Confinement(agent, cwd, drv)
	if err != nil {
		return nil, nil, err
	}
	sandboxed := l.Roster.SandboxOn(agent, cfg)
	var allowWrite, allowDomains []string
	var cacheDir string
	var allowed team.RepoAllowances
	if sandboxed {
		allowWrite, allowDomains, cacheDir = l.SandboxAllowWrite(agent), team.SandboxDomains(cfg), l.SandboxCacheDir(agent)
		allowed = l.Roster.RepoAllowancesOf(cfg)
	}
	granted, err := access.ForLaunch(l.Q, agent, resuming)
	if err != nil {
		return nil, nil, err
	}
	var trusted []string
	if len(mode) > 0 && chosen == team.DefaultMode {
		trusted = l.TrustedEnvironment(cwd)
	}
	argv := drv.LaunchArgv(exe, cli.LaunchArgs{
		Model: modelOf(cfg), Effort: effort, Disallowed: DisallowedTools,
		MCPArgs:  append(append([]string{}, mcpArg...), team.MCPConfigArgs(cfg)...),
		Preamble: preamble, Hide: hide,
		NoWrite:   noWrite,
		MemoryDir: memoryDir, Trusted: trusted, Chrome: team.Chrome(cfg),
		SandboxOff:  !sandboxed,
		MCPApproved: approved, ExtraDirs: extraDirs, ReadAllow: append([]string{filepath.Join(l.Home, "attachments")}, granted...),
		AllowWrite: allowWrite, AllowDomains: allowDomains, CacheDir: cacheDir,
		WeakerNetwork: allowed.WeakerNetwork, Excluded: allowed.Excluded,
	})
	// The pair goes whole or not at all: a caller's own mode wins, and a lone value left behind would be read as the opening prompt.
	if len(mode) > 0 {
		own := false
		for _, x := range extra {
			if x == mode[0] || strings.HasPrefix(x, mode[0]+"=") {
				own = true
			}
		}
		if !own {
			argv = append(argv, mode...)
		}
	}
	argv = append(argv, extra...)

	env := p.Accounts.EnvFrom(site.Env, acct, agent)
	if env == nil {
		env = map[string]string{}
	}
	for k, v := range drv.LaunchEnv(preamble, DisallowedTools, paths.AgentDir(l.Home, agent)) {
		env[k] = v
	}
	return argv, env, nil
}

// keepWhatItStartsWith keeps every session a launch starts (db.session_starts): why, whether it resumes the old conversation (then its
// id is the one after --resume), and what it is given. Bookkeeping never blocks a launch.
func (l *Launcher) keepWhatItStartsWith(site *Site, agent string, extra []string, prompt, why, by string) error {
	resumed, sid := false, ""
	for i, x := range extra {
		if x == "--resume" {
			resumed = true
			if i+1 < len(extra) {
				sid = extra[i+1]
			}
			break
		}
	}
	text, err := db.GetHandoff(l.Q, agent)
	if err != nil {
		return err
	}
	if why == "" {
		why = "launch"
	}
	changed, err := l.whatChanged(agent)
	if err != nil {
		site.warn("hl launch: could not read what changed for %s: %v", agent, err)
	}
	if _, err := db.NoteSessionStart(l.Q, agent, why, resumed, sid, text, prompt, by, changed); err != nil {
		site.warn("hl launch: could not keep how %s was started: %v", agent, err)
	}
	return nil
}

// Prepared is a launch ready to become the agent: the command line and the whole environment, as KEY=VALUE.
type Prepared struct {
	Argv []string
	Env  []string
}

// Prepare builds the launch for site and keeps what it starts with (the session start, the handoff taken, the terminal, the account and
// the fingerprint); the caller locks the worktree and execs. It returns only a *Refusal, or an error when the launch cannot be built.
func (l *Launcher) Prepare(site *Site, agent string, extra []string, force bool, promptFile, account, why, by string) (*Prepared, error) {
	if err := l.refuseOthersPane(site, agent); err != nil {
		return nil, err
	}
	argv, env, err := l.Build(site, agent, append([]string(nil), extra...), force, account)
	if err != nil {
		var refused *Refusal
		if errors.As(err, &refused) {
			if _, aerr := db.AddEvent(l.Q, agent, LaunchRefusedKind, db.KV{K: "why", V: refused.Msg}); aerr != nil {
				return nil, aerr
			}
		}
		return nil, err
	}
	prompt := TakePrompt(promptFile)
	if promptFile == "" && !slices.Contains(extra, "--resume") {
		if prompt, err = l.handLaunchPrompt(agent); err != nil {
			return nil, err
		}
	}
	if err := l.keepWhatItStartsWith(site, agent, extra, prompt, why, by); err != nil {
		return nil, err // Python reads the waiting handoff outside its never-block try
	}
	if prompt != "" {
		drv, err := l.Ports.Vendors.ForAgent(agent)
		if err != nil {
			return nil, err
		}
		argv = append(argv, drv.PromptArgv(prompt)...)
	}
	if promptFile == "" {
		// A launch nobody is waiting on consumed the handoff Build put in the system prompt. The daemon's own wake passes a prompt file and
		// clears the row itself once the session has started, because a launch that dies must leave it for the fallback launch.
		if err := db.SetHandoff(l.Q, agent, ""); err != nil {
			return nil, err
		}
	}
	if !l.Tokens.Record(agent, site.PID) { // the proof this session is the agent's: without it its hl would not be told from the founder's
		return nil, fmt.Errorf("the session of %s could not be recorded, so it is not started", agent)
	}
	fp, _ := l.Fingerprint(agent)
	l.recordTerminal(site, agent, env["HANDLINE_ACCOUNT"], fp)
	gitrun.PointOriginHeadAtMain(site.Cwd) // the session names its main branch from it
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	flat := make([]string, 0, len(keys))
	for _, k := range keys {
		flat = append(flat, k+"="+env[k])
	}
	return &Prepared{Argv: argv, Env: flat}, nil
}

// refuseOthersPane refuses a launch from a pane the daemon opened for another agent: the pane is that agent's terminal, not ours to take.
func (l *Launcher) refuseOthersPane(site *Site, agent string) error {
	if l.Ports.PaneAgent == nil {
		return nil
	}
	handle := site.Handle
	if handle == "" {
		return nil
	}
	if owner := l.Ports.PaneAgent(handle); owner != "" && owner != agent {
		return fmt.Errorf("terminal %s was opened for %s; %s cannot be launched in it", handle, owner, agent)
	}
	return nil
}

// handLaunchPrompt starts the first turn of a fresh session a person launched: without one it sits at an empty prompt, and the daemon
// types nothing to an agent whose task is already in hand.
func (l *Launcher) handLaunchPrompt(agent string) (string, error) {
	handoff, err := db.GetHandoff(l.Q, agent)
	if err != nil {
		return "", err
	}
	if handoff != "" {
		return "This is a fresh session. Your handoff is in your system prompt above; pick the task back up from it.", nil
	}
	tid, held, err := db.CurrentTask(l.Q, agent)
	if err != nil || !held {
		return "", err
	}
	return fmt.Sprintf("This is a fresh session and your context is empty. You hold T%d: read it with `hl task show T%d` and carry on with it.", tid, tid), nil
}

// recordTerminal remembers where we are about to become the agent: the terminal, the account, and what Build produced this launch with.
// The host puts the handle in the environment of every terminal it launches, and this is the only moment it can be seen. A handle or
// worktree carrying a control byte is refused, not rewritten.
func (l *Launcher) recordTerminal(site *Site, agent, acct, fp string) {
	handle, wt := site.Handle, site.WorktreeID
	if handle != "" && textsafe.HasUnsafe(handle) {
		handle = ""
	}
	if wt != "" && textsafe.HasUnsafe(wt) {
		wt = ""
	}
	if handle == "" && acct == "" {
		return
	}
	err := func() error {
		if handle != "" {
			if err := db.SetTerminal(l.Q, agent, handle, wt); err != nil {
				return err
			}
			// However this terminal was made, the agent joins the team's Run here.
			if l.Ports.RegisterWorker != nil {
				if err := l.Ports.RegisterWorker(agent, handle); err != nil {
					return err
				}
			}
			rec, err := l.LaunchRecord(agent)
			if err != nil {
				return err
			}
			if err := db.SetLaunchFingerprint(l.Q, agent, fp, rec); err != nil {
				return err
			}
		}
		if acct != "" {
			return db.SetAccount(l.Q, agent, acct, "")
		}
		return nil
	}()
	if err != nil {
		// Not silent: with no fingerprint on record the daemon relaunches this session at every wake.
		site.warn("hl launch: could not record this launch for %s: %v", agent, err)
	}
}
