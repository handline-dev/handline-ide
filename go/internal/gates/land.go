package gates

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/taskwrites"
	"github.com/handline/handline/go/taskboard/watch"
)

// A gate that closes a task (task.wait then done) does it when its watch passes, as a review turn's pass closes one: in the name of the
// agent acting on the task, through the same done as anyone's, with the merge commit as its evidence and never words. The commit is
// looked for on the default branch of the folder the roster keeps for that agent, fetched first, and nowhere a caller names. What
// cannot be closed goes back to its holder with the reason; what cannot be checked yet keeps the gate open, and the next look tries
// again. The gate's done and the task's done are one write, so no daemon death leaves one without the other. docs/watch-vendors.md
// has each case.

const (
	// FetchWait is how long the fetch of a default branch may take.
	FetchWait = 60 * time.Second
	// UnlandedReads is how many looks in a row may find a merge that origin's default branch does not hold, the fetch having worked,
	// before the task is given back: the host says merged a moment before every reader of the repository sees the commit.
	UnlandedReads = 3
)

// fetchProtocols are the transports the fetch may use (GIT_ALLOW_PROTOCOL): never ext::, which runs a command.
var fetchProtocols = []string{"https", "ssh", "git"}

// daemonAuthor writes the daemon's own notes on a task, as its other parts do.
const daemonAuthor = "daemon"

// ErrTransport is a fetch git refused because the origin's transport is not one the daemon may use (a local path, ext::): no later
// fetch will go better, so the clone is judged as it stands.
var ErrTransport = errors.New("transport not allowed")

// transportRefused is git saying so, in the C locale: "fatal: transport 'file' not allowed".
var transportRefused = regexp.MustCompile(`transport '[^']*' not allowed`)

type transportError struct{ said string }

func (e transportError) Error() string   { return "git fetch: " + e.said }
func (e transportError) Is(t error) bool { return t == ErrTransport }

// FetchBase fetches origin's copy of the default branch of the repository at repo, with no one at it (nothing prompts) and for at most
// FetchWait; also over the further transports named. It is run only in a folder the roster keeps for an agent. An origin whose
// transport is not allowed is ErrTransport.
func FetchBase(repo string, also ...string) error { return fetchBase(repo, FetchWait, also...) }

// fetchBase is FetchBase with its limit given. git runs in its own process group, and the whole group is killed at the limit: git
// starts helpers (ssh, git-remote-https, an upload-pack) that outlive it and hold its output open, and a kill of git alone would wait
// for them. A helper that left the group is waited for one second more, then left behind.
func fetchBase(repo string, limit time.Duration, also ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "fetch", "--quiet", "--no-tags", "origin", gitrun.DefaultBranch(repo))
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), append(gitrun.NoPrompts(append(append([]string{}, fetchProtocols...), also...)), "LC_ALL=C")...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	said := oneLine(strings.TrimSpace(out.String()), "")
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("git fetch timed out after %s", limit)
	case err != nil && transportRefused.MatchString(said):
		return transportError{transportRefused.FindString(said)}
	case err != nil && said == "":
		return fmt.Errorf("git fetch: %s", err)
	case err != nil:
		return fmt.Errorf("git fetch: %s", said)
	}
	return nil
}

// oneLine is s on one line, or other when s is empty.
func oneLine(s, other string) string {
	if s = strings.Join(strings.Fields(textsafe.Oneline(s)), " "); s == "" {
		return other
	}
	return s
}

// closing is what a passed gate does about the task it closes: close it (by, with sha checked in repo), give it back with why it was not
// closed (back), or wait for the next look (wait). A zero closing is a gate that closes nothing: it opens as any gate does.
type closing struct {
	task      int64
	by        string
	sha, repo string
	done      taskwrites.DoneIn // the task's done, its commit already looked up in repo
	back      string
	wait      string
}

// held is what a gate says while its watch has passed with evidence and the close of task waits on why. heldPass reads it back: the
// evidence is kept in it, so a gate that runs out of time while held still opens with the merge it saw, and never as plain timed out.
func held(evidence string, task int64, why string) string {
	return fmt.Sprintf("%s%s; not closing T%d yet: %s", db.PassHeld, evidence, task, why)
}

func heldPass(w db.Watch) (evidence, why string, ok bool) {
	rest, isHeld := strings.CutPrefix(w.Error, db.PassHeld)
	if !isHeld || w.Closes == 0 {
		return "", "", false
	}
	return strings.Cut(rest, fmt.Sprintf("; not closing T%d yet: ", w.Closes))
}

// pastDeadline is what a gate past its deadline opens with. One that was held as passed opens as passed, with the evidence it held, and
// gives its task back saying why it was not closed in time; any other is timed out.
func (s *Sweep) pastDeadline(gate int64, w db.Watch) (evidence, result string, c closing) {
	seen, why, ok := heldPass(w)
	if !ok {
		return "timed out", "timed_out", closing{}
	}
	if t, err := closes(s.DB, gate, w.Closes); err == nil && t != nil {
		c = closing{task: t.ID, by: t.Acting, back: why + " (tried until the gate's deadline)"}
	}
	return seen, string(watch.Passed), c
}

// closes is the task a gate still has to close: open, and still waiting on the gate. One taken off the gate by hand is its holder's
// again, and no longer the gate's to close: nil.
func closes(q db.Q, gate, task int64) (*db.Task, error) {
	t, err := db.GetTask(q, task)
	if err != nil || t == nil || t.Ended() {
		return nil, err
	}
	blockers, err := db.TaskBlockers(q, t.ID)
	if err != nil {
		return nil, err
	}
	for _, b := range blockers {
		if b.ID == gate {
			return t, nil
		}
	}
	return nil, nil
}

// closingOf decides, outside any write, what gate does about the task its watch says to close, now that the watch passed with evidence.
func (s *Sweep) closingOf(gate int64, w db.Watch, evidence string) (closing, error) {
	t, err := closes(s.DB, gate, w.Closes)
	if err != nil || t == nil {
		return closing{}, err
	}
	c := closing{task: t.ID, by: t.Acting}
	sha := watch.MergedSha(evidence)
	switch {
	case actor.IsPerson(t.Acting) || t.Acting == db.Gate:
		c.back = "it is not an agent's to close now"
		return c, nil
	case sha == "":
		c.back = "the merge names no commit to close it with"
		return c, nil
	}
	if c.repo = team.NewRoster(s.Home).RepoOf(t.Acting); c.repo == "" {
		c.back = fmt.Sprintf("%s has no repository on this machine to check the merge commit in", addr.Ref(t.Acting))
		return c, nil
	}
	if why := otherRepo(w, c.repo, t.Acting); why != "" {
		c.back = why
		return c, nil
	}
	fetch := s.Fetch
	if fetch == nil {
		fetch = func(repo string) error { return FetchBase(repo) }
	}
	// unfetched is why the fetch was refused for good: the clone is then checked as it stands, and never fetched again to no use.
	// failed is a fetch that may go better next time: the clone is still looked at, since it may hold the commit already.
	unfetched, failed := "", error(nil)
	if gitrun.HasOrigin(c.repo) {
		switch err := fetch(c.repo); {
		case errors.Is(err, ErrTransport):
			unfetched = err.Error()
		case err != nil:
			failed = err
		}
	}
	base := gitrun.BaseOf(c.repo)
	if ok, _ := base.Reaches(c.repo, sha); ok {
		delete(s.unlanded, gate)
		// git is asked here, with no write open; the done reads these answers and runs no git while it holds the database
		c.sha, c.done = sha, taskwrites.DoneIn{Task: t.ID, Sha: sha, Repo: c.repo}.Looked()
		return c, nil
	}
	if failed != nil {
		delete(s.unlanded, gate)
		c.wait = held(evidence, t.ID, failed.Error())
		return c, nil
	}
	where := "the " + base.Branch + " branch"
	if ref := base.Ref(); ref != "" {
		where = ref
	}
	if s.unlanded[gate]++; s.unlanded[gate] < UnlandedReads {
		c.wait = held(evidence, t.ID, fmt.Sprintf("%s is not on %s here", short(sha), where))
		return c, nil
	}
	delete(s.unlanded, gate)
	c.back = fmt.Sprintf("its merge commit %s is not on %s of %s's repository: it was merged into another branch, or in another repository",
		short(sha), where, addr.Ref(t.Acting))
	if unfetched != "" {
		c.back = fmt.Sprintf("its merge commit %s is not on %s of %s's repository, and the daemon cannot fetch that repository's origin to look again "+
			"(%s: it fetches over %s only)", short(sha), where, addr.Ref(t.Acting), unfetched, strings.Join(fetchProtocols, ", "))
	}
	return c, nil
}

// otherRepo is why a watched pull request's merge is not looked for in the folder at repo, "" when it may be: the pull request is in
// one repository and the folder's origin must be that one. A commit of another repository can be on this one's default branch too (a
// fork, a mirror), and a merge there says nothing of the work here. An origin that is no remote on a host cannot be told to be it. A
// watch that is not a pull request names no repository, and is not held to this.
func otherRepo(w db.Watch, repo, acting string) string {
	pull, _, ok := watch.PullOf(w)
	if !ok {
		return ""
	}
	origin := watch.OriginRepo(gitrun.Run(repo, "remote", "get-url", "origin").Stdout)
	switch {
	case watch.SameRepo(pull, origin):
		return ""
	case origin == "":
		return fmt.Sprintf("its pull request is in %s, and the origin of %s's repository here is no remote on a host, so it cannot be told to be that one",
			pull, addr.Ref(acting))
	}
	return fmt.Sprintf("its pull request is in %s, and %s's repository here is %s: a merge in one is not looked for in the other", pull, addr.Ref(acting), origin)
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// openClosing opens gate as passed and settles the task it closes, in one write: the task is done by c.by with the merge commit, or,
// when that done is refused or c.back says it cannot be, the gate's evidence and a comment on the task say why and the task goes back to
// its holder by the ordinary unblock.
func (s *Sweep) openClosing(gate int64, evidence string, c closing) error {
	if c.back == "" {
		refusal, err := s.closeBoth(gate, evidence, c)
		if err != nil || refusal == "" {
			return err
		}
		c.back = refusal
	}
	return s.giveBack(gate, evidence, c)
}

// errMoved is a task that changed while its merge was being checked, with no write open: it ended, left the gate, or is now another's to
// act on (a person's turn, another agent's). Nothing is written, the gate stays open, and the next look decides again as it stands.
var errMoved = errors.New("the task changed while its merge was checked; the next look decides again")

// closeBoth marks the gate and its task done together. A refusal of the task's done is answered, with nothing written. What closingOf
// decided was read before the fetch, which can take a minute, so the task is read again inside the write: errMoved when it changed.
func (s *Sweep) closeBoth(gate int64, evidence string, c closing) (refusal string, err error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if t, err := closes(tx, gate, c.task); err != nil {
		return "", err
	} else if t == nil || t.Acting != c.by {
		return "", errMoved
	}
	gateDone, err := taskwrites.MarkDone(tx, s.Home, db.Gate, taskwrites.DoneIn{Task: gate, Words: evidence})
	if err != nil {
		return "", err
	}
	if err := db.SetWatchResult(tx, gate, string(watch.Passed)); err != nil {
		return "", err
	}
	done, err := taskwrites.MarkDone(tx, s.Home, c.by, c.done)
	var refused *rpc.Error
	if errors.As(err, &refused) && refused.Code == "refused" {
		return refused.Message, nil
	}
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if s.Tell != nil {
		s.Tell.Done(gate, db.Gate, gateDone.Owner, gateDone.Told, gateDone.Finished)
		s.Tell.Done(c.task, c.by, done.Owner, done.Told, done.Finished)
	}
	s.logf("  T%d: closed by gate T%d with %s", c.task, gate, short(c.sha))
	return "", nil
}

// giveBack opens the gate as passed with why its task was not closed, and leaves that on the task as a comment: the unblock tells its
// holder the gate's evidence, and the comment is there for whoever reads the task later.
func (s *Sweep) giveBack(gate int64, evidence string, c closing) error {
	first, _, _ := strings.Cut(c.back, "\n")
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	done, err := taskwrites.MarkDone(tx, s.Home, db.Gate, taskwrites.DoneIn{Task: gate, Words: fmt.Sprintf("%s; T%d not closed: %s", evidence, c.task, first)})
	if err != nil {
		return err
	}
	if err := db.SetWatchResult(tx, gate, string(watch.Passed)); err != nil {
		return err
	}
	note := fmt.Sprintf("Gate T%d passed (%s) and did not close this task: %s", gate, evidence, c.back)
	if _, err := db.AddMemo(tx, c.task, daemonAuthor, note); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.Tell != nil {
		s.Tell.Done(gate, db.Gate, done.Owner, done.Told, done.Finished)
	}
	s.logf("  T%d: gate T%d passed and did not close it: %s", c.task, gate, first)
	return nil
}
