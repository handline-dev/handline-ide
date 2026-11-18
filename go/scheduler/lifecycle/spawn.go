package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/scheduler/worktreepaths"
	"github.com/handline/handline/go/scheduler/worktreesetup"
)

const (
	// SubmitWaitS is how long to wait for the agent to be ready, and then for its own hook to confirm it got the first task: one number,
	// since they describe the same patience.
	SubmitWaitS = 60
	// StartedTimeoutS is how long to wait for a launched agent to show up in the process table before saying it did not: `hl launch`
	// execs claude, which takes a few seconds to be visible.
	StartedTimeoutS = 20
	// RunKey is the meta name of the orchestration Run this team's work hangs under: one per installation.
	RunKey = "orca:run"
)

// gitTimeout is how long a git command may run; a var so a test can shorten it.
var gitTimeout = 120 * time.Second

// Host is the terminal host's worktree, terminal and Run calls (session/term.py, the part the daemon's terminal channel has not
// brought over yet).
type Host interface {
	Available() bool
	Missing() string
	// OpenTerminal runs cmd in a new terminal in cwd; handle "" with why "" is a terminal started whose handle could not be read.
	OpenTerminal(cmd, cwd, title, agent string) (handle, why string)
	WaitIdle(handle string, timeoutMs int) bool
	Submit(handle, prompt string, timeoutS int)
	HasRuns() bool
	RunCurrent() string
	RunExists(run string) bool
	RunUse(run string)
	RunCreate(title string) string
	TaskCreate(spec, title, displayName, run string) string
	Dispatch(task, handle, run string) string
}

// Spawns is team/spawn.py: starting an agent in a desk it takes from its project's worktree pool (Desks). A new desk and a new branch
// start from the fetched origin/<base>: work lands by push, so the checkout's local <base> may be stale or even another history.
type Spawns struct {
	Q        db.Q
	Host     Host
	Launch   Launch
	Delivery Delivery
	Roster   *team.Roster
	// Desks is the project's worktree pool a spawn takes its desk from; nil refuses every spawn.
	Desks Desks
	Sleep func(time.Duration)
	Now   func() time.Time
	Locks *AgentLocks
	// TrustFolder marks folder trusted in every account agent's vendor may start it on, so the session opens without the folder-trust
	// dialog. nil: nothing is marked.
	TrustFolder func(agent, folder string)
	// OnLaunch is told each time a launch has begun (NoteLaunch); nil tells no one.
	OnLaunch func(agent string)
}

var _ Spawner = (*Spawns)(nil)

// NoteLaunch records that the daemon has begun a launch for agent (the "launching" event, which the view reads as starting) and tells
// on, so the view shows it now: hl launch keeps its own start only once it runs, which under load can be many seconds later.
func NoteLaunch(q db.Q, on func(agent string), agent string) error {
	if _, err := db.AddEvent(q, agent, "launching"); err != nil {
		return err
	}
	if on != nil {
		on(agent)
	}
	return nil
}

// took records that agent's launch went into terminal handle at dir and has begun; an unknown handle ("") records nothing.
func (s *Spawns) took(agent, handle, dir string) error {
	if handle == "" {
		return nil
	}
	if err := db.SetTerminal(s.Q, agent, handle, dir); err != nil {
		return err
	}
	if _, err := s.RegisterWorker(agent, handle); err != nil {
		return err
	}
	return NoteLaunch(s.Q, s.OnLaunch, agent)
}

func (s *Spawns) sleep(d time.Duration) {
	if s.Sleep != nil {
		s.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (s *Spawns) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// confirmSession is "" once a session is actually running for agent; otherwise the warning StartIn and Spawn return instead of
// "started": a terminal handle back from the host is not a session - oh-my-zsh's own update prompt once ate a launch command whole and
// left a bare shell, "started" regardless.
func (s *Spawns) confirmSession(agent string) string {
	if s.Delivery.WaitForSession(agent, StartedTimeoutS, "") {
		return ""
	}
	return fmt.Sprintf("no session is running in it after %ds - the terminal is there and claude is not. Go and look (t): whatever it printed is on that shell.", StartedTimeoutS)
}

// git runs git as (exit code, stdout), stdout cleaned at the source so a caller that embeds it in a message it returns need not
// sanitise it first. The error is git not running at all (not installed, no such directory, timed out), which Python's _run raises.
func git(cwd string, args ...string) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	var out strings.Builder
	cmd.Stdout = &out
	code := 0
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return 0, "", fmt.Errorf("git %s: timed out after %s", strings.Join(args, " "), gitTimeout)
		}
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return 0, "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
	}
	return code, textsafe.Oneline(strings.TrimSpace(out.String())), nil
}

func hasCommit(repo, ref string) (bool, error) {
	code, _, err := git(repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return code == 0 && err == nil, err
}

// PickBase is the ref to branch from and why: origin/<base> once fetched, local <base> only when there is no origin/<base>. An empty ref
// means there is nothing to branch from: a human has to act.
func PickBase(repo, base string) (ref, why string, err error) {
	if _, _, err := git(repo, "fetch", "-q", "origin"); err != nil {
		return "", "", err
	}
	have, err := hasCommit(repo, base)
	if err != nil {
		return "", "", err
	}
	if !have {
		// `git worktree add ... <base>` would die on the name (legacy repos have no main).
		if have, err := hasCommit(repo, "origin/"+base); err != nil || have {
			return "origin/" + base, fmt.Sprintf("no local %s; using origin/%s", base, base), err
		}
		_, head, err := git(repo, "symbolic-ref", "--short", "-q", "HEAD")
		if err != nil {
			return "", "", err
		}
		if head != "" {
			if have, err := hasCommit(repo, head); err != nil || have {
				return head, fmt.Sprintf("no %s here; using the checked-out %s", base, head), err
			}
		}
		return "", fmt.Sprintf("%s has no %s branch and no commit to branch from - make a first commit there", repo, base), nil
	}
	if have, err := hasCommit(repo, "origin/"+base); err != nil || !have {
		return base, fmt.Sprintf("no origin/%s; using local %s", base, base), err
	}
	why = "fetched"
	if code, out, err := git(repo, "rev-list", "--count", fmt.Sprintf("origin/%s..%s", base, base)); err != nil {
		return "", "", err
	} else if n, _ := strconv.Atoi(out); code == 0 && n > 0 {
		why += fmt.Sprintf("; local %s has %d commit(s) not on it, left out", base, n)
	}
	return "origin/" + base, why, nil
}

// pyRepr is repr() of a str, as far as the refusals need it.
func pyRepr(s string) string {
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	if !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

// PromptLanded waits until the agent's own hook reports a submitted prompt. The host reports that input was accepted, which is not the
// same as submitted - for the Claude TUI it cannot observe the difference at all. UserPromptSubmit is the agent itself saying it received
// a task. It matches any prompt for that agent, so a human typing into the terminal first would also satisfy it: the question being
// asked is whether this agent is taking instructions at all.
func (s *Spawns) PromptLanded(agent string, sinceID int64, timeout time.Duration) (bool, error) {
	deadline := s.now().Add(timeout)
	for {
		yes, err := db.HasEventAfter(s.Q, sinceID, agent, "UserPromptSubmit")
		if err != nil || yes {
			return yes, err
		}
		if !s.now().Before(deadline) {
			return false, nil
		}
		s.sleep(2 * time.Second)
	}
}

// Trust marks the folder the daemon is about to start agent in as trusted. Only a folder it launches in is marked, never one a caller names.
func (s *Spawns) Trust(agent, folder string) {
	if s.TrustFolder != nil {
		s.TrustFolder(agent, folder)
	}
}

// WorktreePath is the directory an agent works in, out of what its row holds, "" when it is gone. `hl launch` records what the host
// handed it, which is a selector - the repo's id, then `::`, then the path - while spawn records a plain path. Testing the selector as a
// path finds nothing, so an agent that still had its worktree was given a second one beside it.
func (s *Spawns) WorktreePath(recorded string) string { return worktreepaths.WorktreePath(recorded) }

// WorktreesDir is where repo's desks are (worktreepaths.Dir).
func WorktreesDir(repo string) string { return worktreepaths.Dir(repo) }

// Prepare runs repo's worktree setup (worktreesetup) in wt, a worktree just made for agent, and is what to tell of it: "" when the repo
// asks for nothing. A failed setup is told and recorded, never fatal: the agent starts in the worktree and can finish the setup itself.
func (s *Spawns) Prepare(agent, repo, wt string) string {
	if s.Roster == nil {
		return ""
	}
	setup := worktreesetup.Get(s.Roster.Store, repo)
	if setup.Empty() {
		return ""
	}
	r := worktreesetup.Run(repo, wt, setup, s.now(), worktreesetup.ScriptLimit)
	said := r.Said()
	_, evErr := db.AddEvent(s.Q, agent, "worktree_setup", db.KV{K: "worktree", V: wt}, db.KV{K: "ok", V: strconv.FormatBool(r.OK)}, db.KV{K: "said", V: said})
	if err := errors.Join(worktreesetup.Record(s.Q, repo, r), evErr); err != nil {
		said += fmt.Sprintf(" (not recorded: %v)", err)
	}
	return said
}

// StartIn starts an agent in a worktree it already has. A terminal can be closed by a human, or lost when the host restarts, without
// anything happening to the work: the branch, the checkout and whatever is uncommitted in it are all still there. Making a second
// worktree for that agent would be the wrong repair - it would strand the first. cmd is what the terminal runs; a wake passes its launch
// with the resume id and prompt file, since a closed session's pane is closed with it.
func (s *Spawns) StartIn(agent, worktree, cmd string) (bool, string, error) {
	if !s.Host.Available() {
		return false, s.Host.Missing(), nil
	}
	if _, err := os.Stat(worktree); err != nil {
		return false, fmt.Sprintf("%s: its worktree %s is gone", addr.Ref(agent), worktree), nil
	}
	if cmd == "" {
		var err error
		if cmd, err = s.Launch.CommandFor(agent, "", "", ""); err != nil {
			return false, "", err
		}
	}
	s.Trust(agent, worktree)
	handle, why := s.Host.OpenTerminal(cmd, worktree, agent, agent)
	if handle == "" && why != "" {
		return false, fmt.Sprintf("%s: could not start in %s: %s", addr.Ref(agent), worktree, why), nil
	}
	if err := s.took(agent, handle, worktree); err != nil {
		return false, "", err
	}
	shown := handle
	if shown == "" {
		shown = "unknown"
	}
	said := fmt.Sprintf("%s: started in %s (terminal %s)", addr.Ref(agent), worktree, shown)
	if problem := s.confirmSession(agent); problem != "" {
		return false, said + "\n  but " + problem, nil
	}
	return true, said, nil
}

// SpawnOpts say how Spawn goes. Why is what the session's start is recorded as ("" for hl launch's own default).
type SpawnOpts struct {
	Base, Prompt, Why string
	DryRun            bool
}

// Spawn starts the agent in a desk of repo (SpawnWith).
func (s *Spawns) Spawn(agent, repo string) (bool, string, error) {
	return s.SpawnWith(agent, repo, SpawnOpts{})
}

// SpawnWith starts the agent in the desk of repo the pool gives it (worktreepool.Pool.Take: the one it holds, else its pinned one,
// else its last, else a free one, else a new desk-<n>), with its branch checked out there, a new one from o.Base.
func (s *Spawns) SpawnWith(agent, repo string, o SpawnOpts) (bool, string, error) {
	if !s.Host.Available() {
		return false, s.Host.Missing(), nil
	}
	if s.Roster != nil {
		if _, err := s.Roster.Read(agent); err != nil {
			return false, err.Error(), nil
		}
	}
	if s.Desks == nil {
		return false, "no worktree pool to take a desk from", nil
	}
	cmd, err := s.Launch.CommandFor(agent, "", o.Why, "")
	if err != nil {
		return false, "", err
	}
	if o.DryRun {
		return true, fmt.Sprintf("would take a desk of %s and run there:\n  %s", repo, cmd), nil
	}
	t, why, err := s.Desks.TakeFrom(agent, repo, o.Base)
	if err != nil {
		return false, "", err
	}
	if why != "" {
		return false, fmt.Sprintf("%s: no desk to start in - %s", addr.Ref(agent), why), nil
	}
	wt := t.Path
	s.Trust(agent, wt)
	handle, failed := s.Host.OpenTerminal(cmd, wt, agent, agent)
	if handle == "" && failed != "" {
		return false, fmt.Sprintf("worktree %s is ready, but starting the agent failed: %s", wt, failed), nil
	}
	if err := s.took(agent, handle, wt); err != nil {
		return false, "", err
	}
	shown := handle
	if shown == "" {
		shown = "unknown"
	}
	lines := []string{fmt.Sprintf("%s: desk %s (%s, branch %s)", addr.Ref(agent), wt, t.How, t.Branch)}
	for _, said := range t.Said {
		lines = append(lines, "  "+said)
	}
	lines = append(lines, "  terminal "+shown)
	if problem := s.confirmSession(agent); problem != "" {
		return false, strings.Join(append(lines, "  WARNING: "+problem), "\n"), nil
	}
	if o.Prompt != "" && handle == "" {
		// Sending to a handle we could not read means typing into nothing and then blaming the agent's input box for a failure that
		// happened here.
		lines = append(lines, "  WARNING: could not read the terminal handle, so the first task was not sent. The agent is up; send it by hand.")
		return false, strings.Join(lines, "\n"), nil
	}
	if o.Prompt != "" {
		// Typing into a TUI that is still booting loses the opening characters and leaves the remainder sitting in the input box, unsent:
		// three agents came up that way and did nothing at all, while the host had reported the send as accepted. So wait for the agent
		// to be ready, then believe its own UserPromptSubmit rather than the send receipt.
		since, err := db.MaxEventID(s.Q)
		if err != nil {
			return false, "", err
		}
		// A host without this wait would no-op silently and we would type into a booting TUI: the exact bug this exists to stop.
		if !s.Host.WaitIdle(handle, SubmitWaitS*1000) {
			lines = append(lines, "  WARNING: could not wait for the agent to be ready; the first task may have been typed into a booting TUI")
		}
		s.Host.Submit(handle, o.Prompt, 30)
		landed, err := s.PromptLanded(agent, since, SubmitWaitS*time.Second)
		if err != nil {
			return false, "", err
		}
		if !landed {
			// The agent exists and the worktree is real, but it was asked to do something and never got it. Anything driving spawns from
			// a script reads the exit code, not this text.
			lines = append(lines, fmt.Sprintf("  WARNING: %s never reported receiving the task. It is probably sitting in the input box unsent - check the terminal, clear it with an interrupt and send again.", agent))
			return false, strings.Join(lines, "\n"), nil
		}
		lines = append(lines, "  first task submitted")
	}
	return true, strings.Join(lines, "\n"), nil
}

// EnsureRun is the Run to register workers in, making one the first time. The host draws a Run as a card with its workers under it, so
// this is what turns a pile of terminals into a team anyone can see. It is remembered rather than recreated: a second Run splits the same
// team across two cards, and `--parent` then refuses because the two tasks are in different Runs. A terminal that is already bound to a
// Run wins, because that binding is what a task lands in whatever `--run` says.
func (s *Spawns) EnsureRun() (string, error) {
	run, _, err := db.Meta(s.Q, RunKey)
	if err != nil {
		return "", err
	}
	if bound := s.Host.RunCurrent(); bound != "" {
		if bound != run {
			if err := db.SetMeta(s.Q, RunKey, bound); err != nil {
				return "", err
			}
		}
		return bound, nil
	}
	if run != "" && s.Host.RunExists(run) {
		s.Host.RunUse(run)
		return run, nil
	}
	if run = s.Host.RunCreate("Handline team"); run != "" {
		return run, db.SetMeta(s.Q, RunKey, run)
	}
	return "", nil
}

// RegisterWorker makes this agent's terminal a worker in the Run, under its supervisor. Without it an agent is one more tab: the host
// has no idea it belongs to a team, which is why an agent asked to get work done reached for a subagent - the only child it knew how to
// make. The tree comes from the roster, so the card and the routing cannot disagree about who reports to whom. "" when nothing was
// registered.
func (s *Spawns) RegisterWorker(agent, handle string) (string, error) {
	if !s.Host.HasRuns() {
		return "", nil
	}
	run, err := s.EnsureRun()
	if err != nil || run == "" {
		return "", err
	}
	row, err := db.GetAgentState(s.Q, agent)
	if err != nil {
		return "", err
	}
	if row != nil && row.Task.String != "" {
		// Already a worker; the dispatch binds the (possibly new) terminal.
		return s.Host.Dispatch(row.Task.String, handle, run), nil
	}
	spec := fmt.Sprintf("%s: a Handline agent. Work reaches it as a task (`hl task new --assign %s`); this task is the card it appears in.", agent, agent)
	task := s.Host.TaskCreate(spec, agent, agent, run)
	if task == "" {
		return "", nil
	}
	if err := db.SetTask(s.Q, agent, task); err != nil {
		return "", err
	}
	return s.Host.Dispatch(task, handle, run), nil
}
