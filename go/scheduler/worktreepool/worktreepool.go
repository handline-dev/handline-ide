// Package worktreepool keeps a project's desks: the worktree folders under <repo>.worktrees.noindex that agents take when a session
// starts and give back when it closes. A desk stays when it is given back, so the next agent starts in a warm folder; the work stays on
// the agent's own branch, which every desk shares through the one repository. The folders are worktree_folders' rows; the one an agent
// holds or last held is its agent_state.worktree, and its branch agent_state.branch.
package worktreepool

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/scheduler/worktreelock"
	"github.com/handline/handline/go/scheduler/worktreepaths"
)

// Founder is the pin of a desk kept for the founder's own work beside an agent: no agent takes it.
const Founder = "founder"

// DeskPrefix names a desk: desk-1, desk-2, ...
const DeskPrefix = "desk-"

// gitLimit is how long one checkout or worktree add may take.
const gitLimit = 120 * time.Second

// Pool takes and gives back desks. Its calls are serialized: two takes never get the same desk.
type Pool struct {
	DB db.Q
	// Base is the ref a new desk and a new branch start from (lifecycle.PickBase over branch, "" the repo's default branch), and why it
	// was chosen.
	Base func(repo, branch string) (ref, why string, err error)
	// Branch is the name of a new branch for agent, the first time it takes a desk without one of its own.
	Branch func(agent string) string
	// Setup runs the project's setup in a desk just made, and is what to tell of it ("" for nothing). nil runs none.
	Setup func(agent, repo, desk string) string
	// Running is whether a session still runs in desk (git's worktree lock, worktreelock.Held): no take or give back touches it then.
	// nil is never.
	Running func(repo, desk string) bool
	Now     func() time.Time

	mu sync.Mutex
}

func (p *Pool) running(repo, desk string) bool { return p.Running != nil && p.Running(repo, desk) }

func (p *Pool) now() int64 {
	if p.Now != nil {
		return p.Now().Unix()
	}
	return time.Now().Unix()
}

// Folder is one desk.
type Folder struct {
	Path, Repo, Holder, LastAgent, PinnedTo string
	LastUsed                                int64
}

// Taken is the desk a take gave: How is "held" (it held it already), "pinned", "last", "free" or "new", or "own" for a folder outside
// the pool the agent keeps; Said is what to tell.
type Taken struct {
	Path, Branch, How string
	Said              []string
}

// Take gives agent a desk of repo and checks out its branch there: the desk it holds, else one pinned to it, else the one it used last
// when free, else the free desk used longest ago, else a new desk-<n> with the lowest free number. A free desk with changes in it is
// passed over. why is set, and the desk not taken, when the branch cannot be checked out (git says why).
func (p *Pool) Take(agent, repo string) (t Taken, why string, err error) {
	return p.TakeFrom(agent, repo, "")
}

// TakeFrom is Take with a new desk and a new branch started from base ("" the repo's default branch).
func (p *Pool) TakeFrom(agent, repo, base string) (t Taken, why string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	repo = canonical(repo)
	if err := p.forgetGone(repo); err != nil {
		return t, "", err
	}
	folders, err := List(p.DB, repo)
	if err != nil {
		return t, "", err
	}
	last, branch, err := p.agentRow(agent)
	if err != nil {
		return t, "", err
	}
	pick := func(how string, ok func(f Folder) bool) bool {
		for _, f := range folders {
			if ok(f) && exists(f.Path) && (f.Holder == agent || clean(f.Path) && !p.running(repo, f.Path)) {
				t.Path, t.How = f.Path, how
				return true
			}
		}
		return false
	}
	free := func(f Folder) bool { return f.Holder == "" && (f.PinnedTo == "" || f.PinnedTo == agent) }
	if last != "" && exists(last) && !known(folders, last) {
		// a folder outside the pool (the repository's own checkout, or one elsewhere) stays the agent's: the pool hands out desks only
		return Taken{Path: last, Branch: head(last), How: "own"}, "", nil
	}
	switch {
	case pick("held", func(f Folder) bool { return f.Holder == agent }):
	case pick("pinned", func(f Folder) bool { return f.Holder == "" && f.PinnedTo == agent }):
	case last != "" && pick("last", func(f Folder) bool { return free(f) && samePath(f.Path, last) }):
	case pick("free", free): // List is oldest first
	default:
		desk, said, why, err := p.makeDesk(agent, repo, base, folders)
		if err != nil || why != "" {
			return t, why, err
		}
		t.Path, t.How, t.Said = desk, "new", said
	}
	if t.How == "held" {
		if t.Branch = head(t.Path); t.Branch == "" {
			t.Branch = branch // its own desk as it left it; a detached HEAD there does not lose the branch on record
		}
	} else if t.Branch, why = p.checkout(agent, repo, base, t.Path, branch); why != "" {
		return Taken{}, why, nil
	}
	if _, err := p.DB.Exec("UPDATE worktree_folders SET holder = ?, untracked_told = 0, last_agent = ?, last_used = ? WHERE path = ?",
		agent, agent, p.now(), t.Path); err != nil {
		return t, "", err
	}
	return t, "", p.setAgent(agent, t.Path, t.Branch)
}

// makeDesk adds desk-<n>, n the lowest number no folder of repo has, at the base ref, and runs the project's setup in it.
func (p *Pool) makeDesk(agent, repo, base string, folders []Folder) (desk string, said []string, why string, err error) {
	if p.Base == nil {
		return "", nil, "no base to make a desk from", nil
	}
	ref, how, err := p.Base(repo, base)
	if err != nil || ref == "" {
		return "", nil, how, err
	}
	desk = p.freeDesk(repo, folders)
	if r := gitrun.RunTimeout(repo, gitLimit, "worktree", "add", "--detach", desk, ref); r.RC != 0 {
		return "", nil, fmt.Sprintf("could not make %s from %s: %s", desk, ref, strings.TrimSpace(r.Stderr)), nil
	}
	desk = canonical(desk)
	if _, err := p.DB.Exec("INSERT INTO worktree_folders(path, repo, last_used) VALUES(?,?,?)", desk, repo, p.now()); err != nil {
		return "", nil, "", err
	}
	said = append(said, fmt.Sprintf("made %s from %s (%s)", desk, ref, how))
	if p.Setup != nil {
		if s := p.Setup(agent, repo, desk); s != "" {
			said = append(said, s)
		}
	}
	return desk, said, "", nil
}

// freeDesk is the path of desk-<n> for repo, n the lowest number no folder of repo has and no folder there takes.
func (p *Pool) freeDesk(repo string, folders []Folder) string {
	used := map[string]bool{}
	for _, f := range folders {
		used[filepath.Base(f.Path)] = true
	}
	dir := worktreepaths.Dir(repo)
	for n := 1; ; n++ {
		name := DeskPrefix + strconv.Itoa(n)
		if used[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			continue // a folder the pool does not know: never taken over
		}
		return filepath.Join(dir, name)
	}
}

// checkout puts agent's branch in desk: the one it has, else a new one from the base. A branch checked out in another desk is not
// forced: git refuses, and that is the why.
func (p *Pool) checkout(agent, repo, base, desk, branch string) (string, string) {
	if branch != "" && head(desk) == branch {
		return branch, ""
	}
	if branch != "" && gitrun.Run(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).RC == 0 {
		if r := gitrun.RunTimeout(desk, gitLimit, "checkout", "-q", branch); r.RC != 0 {
			return "", fmt.Sprintf("could not check out %s in %s: %s", branch, desk, strings.TrimSpace(r.Stderr))
		}
		return branch, ""
	}
	if branch == "" {
		// none on record (before the pool, an agent's branch was named after its key): an existing one is its own, never left behind
		for _, b := range []string{agent, agentBranch(p, agent)} {
			if gitrun.Run(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+b).RC == 0 {
				return p.checkout(agent, repo, base, desk, b)
			}
		}
		branch = agentBranch(p, agent)
	}
	if p.Base == nil {
		return "", "no base to start " + branch + " from"
	}
	ref, how, err := p.Base(repo, base)
	if err != nil || ref == "" {
		if err != nil {
			how = err.Error()
		}
		return "", fmt.Sprintf("no base for %s: %s", branch, how)
	}
	if r := gitrun.RunTimeout(desk, gitLimit, "checkout", "-q", "-b", branch, ref); r.RC != 0 {
		return "", fmt.Sprintf("could not start %s from %s in %s: %s", branch, ref, desk, strings.TrimSpace(r.Stderr))
	}
	return branch, ""
}

// Given is what a give back did: Freed is false while untracked files keep the desk held; Committed is the WIP commit made, "" for
// none; Tell is the one line to type to the agent ("" once told in this hold, or with nothing to tell).
type Given struct {
	Desk, Committed, Tell string
	Freed                 bool
	Untracked             []string
}

// GiveBack frees the desk agent holds. Changes to tracked files are committed on its branch first as "WIP at close (T<task>)", never
// pushed; a detached HEAD gets the agent's branch before that commit. Untracked files keep the desk held: they would be lost to, or
// mixed into, the next agent's work, so the agent is told once per hold to commit or delete them.
func (p *Pool) GiveBack(agent string, task int64) (g Given, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.giveBack(agent, task, "SELECT path, repo, untracked_told FROM worktree_folders WHERE holder = ?", agent)
}

// giveBack frees the desk query finds (path, repo, untracked_told), under p.mu.
func (p *Pool) giveBack(agent string, task int64, query string, args ...any) (g Given, err error) {
	var path, repo string
	var told int
	switch err := p.DB.QueryRow(query, args...).Scan(&path, &repo, &told); {
	case err == sql.ErrNoRows:
		return g, nil
	case err != nil:
		return g, err
	}
	g.Desk = path
	if !exists(path) {
		g.Freed = true // its folder is gone: it leaves the pool, and git forgets it so its branch can be checked out elsewhere
		gitrun.Run(repo, "worktree", "prune")
		_, err = p.DB.Exec("DELETE FROM worktree_folders WHERE path = ?", path)
		return g, err
	}
	if p.running(repo, path) {
		return g, nil
	}
	{
		tracked, untracked, why := status(path)
		if why != "" {
			return g, fmt.Errorf("%s: %s", path, why)
		}
		if op := midOperation(path); op != "" {
			untracked = []string{op} // a rebase, merge or cherry-pick half done is the agent's to finish or abort, never a WIP commit
		}
		if len(untracked) > 0 {
			g.Untracked = untracked
			if told == 0 {
				g.Tell = fmt.Sprintf("Your desk %s has work git cannot simply commit (%s); it stays yours until you commit, finish or delete it.",
					path, strings.Join(firstN(untracked, 5), ", "))
				_, err = p.DB.Exec("UPDATE worktree_folders SET untracked_told = 1 WHERE path = ?", path)
			}
			return g, err
		}
		if tracked {
			if g.Committed, err = p.wip(agent, path, task); err != nil {
				return g, err
			}
		}
		if b := head(path); b != "" {
			if _, err := p.DB.Exec("UPDATE agent_state SET branch = ? WHERE agent = ?", b, agent); err != nil {
				return g, err
			}
			// a free desk holds no branch: git lets a branch be checked out in one worktree only, and the agent's next desk may be another
			if r := gitrun.Run(path, "checkout", "-q", "--detach"); r.RC != 0 {
				return g, fmt.Errorf("%s: could not let go of %s: %s", path, b, strings.TrimSpace(r.Stderr))
			}
		}
	}
	g.Freed = true
	_, err = p.DB.Exec("UPDATE worktree_folders SET holder = NULL, untracked_told = 0, last_agent = ?, last_used = ? WHERE path = ?",
		agent, p.now(), path)
	return g, err
}

// wip commits every tracked change in desk on agent's branch and is the commit's id.
func (p *Pool) wip(agent, desk string, task int64) (string, error) {
	if head(desk) == "" {
		_, branch, err := p.agentRow(agent)
		if err != nil {
			return "", err
		}
		if branch == "" || gitrun.Run(desk, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).RC == 0 {
			branch = fmt.Sprintf("%s-wip-%d", agentBranch(p, agent), p.now()) // never moves a branch the detached HEAD is not on
		}
		if r := gitrun.Run(desk, "checkout", "-q", "-b", branch); r.RC != 0 {
			return "", fmt.Errorf("%s: no branch for the WIP commit: %s", desk, strings.TrimSpace(r.Stderr))
		}
	}
	msg := "WIP at close"
	if task > 0 {
		msg = fmt.Sprintf("WIP at close (T%d)", task)
	}
	if r := gitrun.Run(desk, "add", "-u"); r.RC != 0 {
		return "", fmt.Errorf("%s: git add -u: %s", desk, strings.TrimSpace(r.Stderr))
	}
	if r := gitrun.RunTimeout(desk, gitLimit, "commit", "-q", "--no-verify", "-m", msg); r.RC != 0 {
		return "", fmt.Errorf("%s: WIP commit: %s", desk, strings.TrimSpace(r.Stderr+r.Stdout))
	}
	return strings.TrimSpace(gitrun.Run(desk, "rev-parse", "--short", "HEAD").Stdout), nil
}

func agentBranch(p *Pool, agent string) string {
	if p.Branch != nil {
		return p.Branch(agent)
	}
	return agent
}

// Pin keeps desk for who (an agent key or Founder) and nobody else; "" lets it go to anyone.
func Pin(q db.Q, desk, who string) error {
	r, err := q.Exec("UPDATE worktree_folders SET pinned_to = ? WHERE path = ?", nullStr(who), canonical(desk))
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return fmt.Errorf("%s is not a desk", desk)
	}
	return nil
}

// Adopt makes every worktree of repo under its worktree folders a desk, as it is (no rename: a session may resume in it). One an agent
// records as its worktree is held by that agent, and its branch becomes the agent's; one no agent records (the founder's own, a retired
// agent's) is pinned to the founder, who lets it go with hl desk unpin.
func (p *Pool) Adopt(repo string) (added int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	repo = canonical(repo)
	r := gitrun.Run(repo, "worktree", "list", "--porcelain")
	if r.RC != 0 {
		return 0, fmt.Errorf("%s: git worktree list: %s", repo, strings.TrimSpace(r.Stderr))
	}
	owners, err := p.recorded()
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(r.Stdout, "\n") {
		path, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		path = canonical(path)
		if parent := filepath.Dir(path); parent != canonical(worktreepaths.Dir(repo)) && parent != canonical(repo+".worktrees") {
			continue
		}
		holder, pin := owners[path], ""
		if holder == "" {
			pin = Founder
		}
		res, err := p.DB.Exec("INSERT INTO worktree_folders(path, repo, holder, last_agent, last_used, pinned_to) VALUES(?,?,?,?,?,?) "+
			"ON CONFLICT(path) DO NOTHING", path, repo, nullStr(holder), nullStr(holder), p.now(), nullStr(pin))
		if err != nil {
			return added, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		added++
		if b := head(path); holder != "" && b != "" {
			if _, err := p.DB.Exec("UPDATE agent_state SET branch = ? WHERE agent = ? AND branch IS NULL", b, holder); err != nil {
				return added, err
			}
		}
	}
	return added, nil
}

// recorded is each agent's worktree, by real path.
func (p *Pool) recorded() (map[string]string, error) {
	rows, err := p.DB.Query("SELECT agent, COALESCE(worktree, '') FROM agent_state")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	first := map[string]string{}
	for rows.Next() {
		var agent, wt string
		if err := rows.Scan(&agent, &wt); err != nil {
			return nil, err
		}
		if path := worktreepaths.RecordedPath(wt); path != "" {
			first[canonical(path)] = agent
		}
	}
	return first, rows.Err()
}

// Rename moves each free desk of repo not named desk-<n> (from before the pool) to the lowest free desk-<n> with git worktree move; its
// rows follow. A held, pinned, running or resumable one waits for a later round. renamed is old -> new.
func (p *Pool) Rename(repo string) (renamed [][2]string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	repo = canonical(repo)
	folders, err := List(p.DB, repo)
	if err != nil {
		return nil, err
	}
	resumable, err := p.resumable()
	if err != nil {
		return nil, err
	}
	for _, f := range folders {
		if isDeskName(filepath.Base(f.Path)) || f.Holder != "" || f.PinnedTo != "" || resumable[f.Path] || !exists(f.Path) ||
			p.running(repo, f.Path) {
			continue
		}
		desk := p.freeDesk(repo, folders)
		// a lock left by a session that is gone refuses the move
		if _, lerr := worktreelock.Release(repo, f.Path, worktreelock.Running); lerr != nil {
			err = errors.Join(err, lerr)
			continue
		}
		if r := gitrun.RunTimeout(repo, gitLimit, "worktree", "move", f.Path, desk); r.RC != 0 {
			err = errors.Join(err, fmt.Errorf("%s: git worktree move to %s: %s", f.Path, desk, strings.TrimSpace(r.Stderr)))
			continue
		}
		desk = canonical(desk)
		if _, err := p.DB.Exec("UPDATE worktree_folders SET path = ? WHERE path = ?", desk, f.Path); err != nil {
			return renamed, err
		}
		if err := p.moveRecorded(f.Path, desk); err != nil {
			return renamed, err
		}
		folders = append(folders, Folder{Path: desk})
		renamed = append(renamed, [2]string{f.Path, desk})
	}
	return renamed, err
}

// resumable is every folder a closed session may resume in, by real path.
func (p *Pool) resumable() (map[string]bool, error) {
	rows, err := p.DB.Query("SELECT COALESCE(worktree, '') FROM agent_state WHERE COALESCE(resume_session, '') != ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var wt string
		if err := rows.Scan(&wt); err != nil {
			return nil, err
		}
		if path := worktreepaths.RecordedPath(wt); path != "" {
			out[canonical(path)] = true
		}
	}
	return out, rows.Err()
}

// moveRecorded points every agent that records from (its last folder) at to.
func (p *Pool) moveRecorded(from, to string) error {
	rows, err := p.DB.Query("SELECT agent, worktree FROM agent_state WHERE worktree IS NOT NULL")
	if err != nil {
		return err
	}
	var agents []string
	for rows.Next() {
		var agent, wt string
		if err := rows.Scan(&agent, &wt); err != nil {
			rows.Close()
			return err
		}
		if path := worktreepaths.RecordedPath(wt); path != "" && canonical(path) == from {
			agents = append(agents, agent)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, a := range agents {
		if _, err := p.DB.Exec("UPDATE agent_state SET worktree = ? WHERE agent = ?", to, a); err != nil {
			return err
		}
	}
	return nil
}

// isDeskName is whether name is desk-<n>.
func isDeskName(name string) bool {
	n, ok := strings.CutPrefix(name, DeskPrefix)
	return ok && n != "" && strings.Trim(n, "0123456789") == ""
}

// forgetGone takes out of the pool every desk of repo whose folder is gone, and has git forget it, so its branch can be checked out
// in another desk and its holder is not left holding two.
func (p *Pool) forgetGone(repo string) error {
	folders, err := List(p.DB, repo)
	if err != nil {
		return err
	}
	pruned := false
	for _, f := range folders {
		if exists(f.Path) {
			continue
		}
		if !pruned {
			gitrun.Run(repo, "worktree", "prune")
			pruned = true
		}
		if _, err := p.DB.Exec("DELETE FROM worktree_folders WHERE path = ?", f.Path); err != nil {
			return err
		}
	}
	return nil
}

// midOperation names a rebase, merge, cherry-pick or revert left half done in desk, "" for none.
func midOperation(desk string) string {
	for _, op := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		r := gitrun.Run(desk, "rev-parse", "--git-path", op)
		path := strings.TrimSpace(r.Stdout)
		if r.RC != 0 || path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(desk, path)
		}
		if _, err := os.Stat(path); err == nil {
			return "a " + strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(op, "_HEAD"), "-merge")) + " in progress"
		}
	}
	return ""
}

// List is repo's desks ("" for every repo's), the one used longest ago first.
func List(q db.Q, repo string) ([]Folder, error) {
	query, args := "SELECT path, repo, COALESCE(holder, ''), COALESCE(last_agent, ''), COALESCE(pinned_to, ''), last_used FROM worktree_folders", []any{}
	if repo != "" {
		query, args = query+" WHERE repo = ?", append(args, canonical(repo))
	}
	return db.Collect[Folder](q.Query(query+" ORDER BY last_used, path", args...))(func(rows *sql.Rows) (f Folder, err error) {
		return f, rows.Scan(&f.Path, &f.Repo, &f.Holder, &f.LastAgent, &f.PinnedTo, &f.LastUsed)
	})
}

func (p *Pool) agentRow(agent string) (last, branch string, err error) {
	var wt, b sql.NullString
	err = p.DB.QueryRow("SELECT worktree, branch FROM agent_state WHERE agent = ?", agent).Scan(&wt, &b)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return worktreepaths.RecordedPath(wt.String), b.String, err
}

func (p *Pool) setAgent(agent, desk, branch string) error {
	_, err := p.DB.Exec("INSERT INTO agent_state(agent, state, updated, worktree, branch) VALUES(?,?,?,?,?) "+
		"ON CONFLICT(agent) DO UPDATE SET worktree = excluded.worktree, branch = excluded.branch", agent, "IDLE", p.now(), desk, nullStr(branch))
	return err
}

// status is whether desk has changes to tracked files, and its untracked files (ignored ones are not listed).
func status(desk string) (tracked bool, untracked []string, why string) {
	r := gitrun.Run(desk, "status", "--porcelain", "-z")
	if r.RC != 0 {
		return false, nil, "git status: " + strings.TrimSpace(r.Stderr)
	}
	entries := strings.Split(r.Stdout, "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		if strings.HasPrefix(e, "?? ") {
			untracked = append(untracked, e[3:])
			continue
		}
		tracked = true
		if e[0] == 'R' || e[0] == 'C' {
			i++ // a rename or copy's source path follows as its own entry
		}
	}
	return tracked, untracked, ""
}

func clean(desk string) bool {
	tracked, untracked, why := status(desk)
	return why == "" && !tracked && len(untracked) == 0
}

// head is the branch checked out in desk, "" for a detached HEAD.
func head(desk string) string {
	r := gitrun.Run(desk, "symbolic-ref", "--short", "-q", "HEAD")
	if r.RC != 0 {
		return ""
	}
	return strings.TrimSpace(r.Stdout)
}

func exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func known(folders []Folder, path string) bool {
	for _, f := range folders {
		if samePath(f.Path, path) {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool { return canonical(a) == canonical(b) }

// canonical is p with its links resolved (macOS's /var is /private/var), or p as it is when it cannot be.
func canonical(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return append(s[:n:n], fmt.Sprintf("and %d more", len(s)-n))
	}
	return s
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// GraceS is how long after a take a closed agent keeps its desk: a wake takes the desk before its session starts, and the sweep must not
// give it back in between.
const GraceS = 600

// Sweep gives back the desk of every agent whose session is closed, once its process has let go of the folder (Running) and GraceS has
// passed since the take. tell is the one line for the agent when untracked files keep the desk.
func (p *Pool) Sweep(now int64, tell func(agent, text string)) error {
	rows, err := p.DB.Query("SELECT f.path, f.repo, f.holder, COALESCE(s.current_task, 0) FROM worktree_folders f JOIN agent_state s ON s.agent = f.holder "+
		"WHERE COALESCE(s.closed_at, 0) != 0 AND f.last_used < ?", now-GraceS)
	if err != nil {
		return err
	}
	type held struct {
		desk, repo, agent string
		task              int64
	}
	var all []held
	for rows.Next() {
		var h held
		if err := rows.Scan(&h.desk, &h.repo, &h.agent, &h.task); err != nil {
			rows.Close()
			return err
		}
		all = append(all, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var errs []error
	for _, h := range all {
		p.mu.Lock()
		// read again under the lock: a wake may have taken the desk (last_used) or started its session since the list above
		g, err := p.giveBack(h.agent, h.task, "SELECT f.path, f.repo, f.untracked_told FROM worktree_folders f JOIN agent_state s ON s.agent = f.holder "+
			"WHERE f.path = ? AND f.holder = ? AND COALESCE(s.closed_at, 0) != 0 AND f.last_used < ?", h.desk, h.agent, now-GraceS)
		p.mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", h.agent, err))
			continue
		}
		if g.Tell != "" && tell != nil {
			tell(h.agent, g.Tell)
		}
	}
	return errors.Join(errs...)
}

// Find is the desk named by its path or its folder's name (desk-3); a name two projects share is not enough.
func Find(q db.Q, desk string) (string, error) {
	if filepath.IsAbs(desk) {
		var path string
		if err := q.QueryRow("SELECT path FROM worktree_folders WHERE path = ?", canonical(desk)).Scan(&path); err != nil {
			return "", fmt.Errorf("%s is not a desk", desk)
		}
		return path, nil
	}
	rows, err := q.Query("SELECT path FROM worktree_folders")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return "", err
		}
		if filepath.Base(path) == desk {
			found = append(found, path)
		}
	}
	switch {
	case rows.Err() != nil:
		return "", rows.Err()
	case len(found) == 0:
		return "", fmt.Errorf("no desk is named %s", desk)
	case len(found) > 1:
		return "", fmt.Errorf("%d desks are named %s: give its path", len(found), desk)
	}
	return found[0], nil
}
