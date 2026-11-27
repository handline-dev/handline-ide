// Package worktreepool keeps each agent's worktree: one folder per agent under <repo>.worktrees.noindex, made the first time it starts and
// never lent or renamed, since Claude keeps a conversation under the folder it runs in. The folders are worktree_folders' rows, held by
// their agent for good; the one an agent works in is its agent_state.worktree, and its branch agent_state.branch. With Remove on, a closed
// agent's folder is deleted and made again at the same path when it wakes.
package worktreepool

import (
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/scheduler/worktreelock"
	"github.com/handline/handline/go/scheduler/worktreepaths"
)

// Founder is the pin of a folder kept for the founder's own work beside an agent: no agent takes it.
const Founder = "founder"

// RemoveOnSleepKey is the setting that deletes a closed agent's worktree (Pool.RemoveOnSleep); off by default.
const RemoveOnSleepKey = "worktree_remove_on_sleep"

// gitLimit is how long one checkout or worktree add may take.
const gitLimit = 120 * time.Second

// Pool gives each agent its own worktree. Its calls are serialized: two takes never get the same folder.
type Pool struct {
	DB db.Q
	// Base is the ref a new desk and a new branch start from (lifecycle.PickBase over branch, "" the repo's default branch), and why it
	// was chosen.
	Base func(repo, branch string) (ref, why string, err error)
	// Branch is the name of a new branch for agent, the first time it takes a desk without one of its own.
	Branch func(agent string) string
	// Name is the word a new folder of agent's starts with (its name when the folder is made); nil is its key.
	Name func(agent string) string
	// RemoveOnSleep is whether a closed agent's folder is deleted (the setting); nil is never.
	RemoveOnSleep func() bool
	// Making is told before a folder of agent's is made, new or again; nil tells nobody.
	Making func(agent string)
	// Setup runs the project's setup in a desk just made, and is what to tell of it ("" for nothing). nil runs none.
	Setup func(agent, repo, desk string) string
	// Running is whether a session still runs in desk (git's worktree lock, worktreelock.Held): no take or give back touches it then.
	// nil is never.
	Running func(repo, desk string) bool
	Now     func() time.Time

	mu sync.Mutex
}

func (p *Pool) running(repo, desk string) bool { return p.Running != nil && p.Running(repo, desk) }

func (p *Pool) removeOnSleep() bool { return p.RemoveOnSleep != nil && p.RemoveOnSleep() }

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

// Taken is the folder a take gave: How is "held" (its own, there), "last" (the shared pool's free desk it used last), "made again" (its
// own, deleted while it slept), "new", "own" for a folder outside the pool the agent keeps, or "plain folder"; Said is what to tell.
type Taken struct {
	Path, Branch, How string
	Said              []string
}

// plain is whether folder is there and no git checkout's top.
func plain(folder string) bool {
	if fi, err := os.Stat(folder); err != nil || !fi.IsDir() {
		return false
	}
	_, err := os.Stat(filepath.Join(folder, ".git"))
	return errors.Is(err, os.ErrNotExist)
}

// Take gives agent its own worktree of repo with its branch checked out: the one it holds (made again at the same path when it was
// deleted), else the free one it used last, else a new <name>-<hex>. It never takes another agent's. why is set, and nothing taken, when
// the branch cannot be checked out (git says why).
func (p *Pool) Take(agent, repo string) (t Taken, why string, err error) {
	return p.TakeFrom(agent, repo, "")
}

// TakeFrom is Take with a new folder and a new branch started from base ("" the repo's default branch). A plain folder (no git) is
// no pool: it is taken as it is, with no branch, and base is not read.
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
	if last != "" && exists(last) && !known(folders, last) {
		// a folder outside the pool (the repository's own checkout, or one elsewhere) stays the agent's: the pool makes worktrees only
		return Taken{Path: last, Branch: head(last), How: "own"}, "", nil
	}
	if plain(repo) { // no repo to make a worktree of: the agent works in the folder itself, on no branch
		return Taken{Path: repo, How: "plain folder"}, "", nil
	}
	switch f := mine(folders, agent, last); {
	case f != nil && exists(f.Path) && f.Holder == agent:
		t.Path, t.How = f.Path, "held"
	case f != nil && exists(f.Path):
		t.Path, t.How = f.Path, "last" // a free desk of the shared pool it used last: detached when let go
	case f != nil:
		said, why, err := p.make(agent, repo, base, branch, f.Path)
		if err != nil || why != "" {
			return t, why, err
		}
		t.Path, t.How, t.Said = f.Path, "made again", said
	default:
		path, err := p.newPath(agent, repo)
		if err != nil {
			return t, "", err
		}
		said, why, err := p.make(agent, repo, base, branch, path)
		if err != nil || why != "" {
			return t, why, err
		}
		path = canonical(path)
		if _, err := p.DB.Exec("INSERT INTO worktree_folders(path, repo, last_used) VALUES(?,?,?)", path, repo, p.now()); err != nil {
			return t, "", err
		}
		t.Path, t.How, t.Said = path, "new", said
	}
	switch t.Branch = head(t.Path); {
	case t.Branch != "":
	case t.How == "held":
		// its own folder as it left it: a detached HEAD there (a rebase or a bisect under way) is the agent's, and its branch stays on
		// record
		t.Branch = branch
	default:
		if t.Branch, why = p.checkout(agent, repo, base, t.Path, branch); why != "" {
			p.unmake(repo, t)
			return Taken{}, why, nil
		}
	}
	if _, err := p.DB.Exec("UPDATE worktree_folders SET holder = ?, last_agent = ?, last_used = ? WHERE path = ?",
		agent, agent, p.now(), t.Path); err != nil {
		return t, "", err
	}
	return t, "", p.setAgent(agent, t.Path, t.Branch)
}

// mine is agent's folder among folders: the one it holds (the one it records first), else the free one it records and used last, which
// nobody is pinned to but it. nil when it has none: another agent's folder is never its.
func mine(folders []Folder, agent, last string) *Folder {
	var held *Folder
	for i, f := range folders {
		if f.Holder != agent {
			continue
		}
		if held == nil || samePath(f.Path, last) {
			held = &folders[i]
		}
	}
	if held != nil {
		return held
	}
	for i, f := range folders {
		if last != "" && samePath(f.Path, last) && f.Holder == "" && (f.LastAgent == "" || f.LastAgent == agent) &&
			(f.PinnedTo == "" || f.PinnedTo == agent) {
			return &folders[i]
		}
	}
	return nil
}

// unmake takes away a folder t made whose branch could not be checked out, so the next take makes it again rather than leave it
// behind: a new one leaves the pool too.
func (p *Pool) unmake(repo string, t Taken) {
	if t.How != "new" && t.How != "made again" {
		return
	}
	gitrun.RunTimeout(repo, gitLimit, "worktree", "remove", "--force", t.Path)
	if t.How == "new" {
		p.DB.Exec("DELETE FROM worktree_folders WHERE path = ?", t.Path)
	}
}

// newPath is a folder for agent no one has: <name>-<six hex digits> beside repo's other worktrees.
func (p *Pool) newPath(agent, repo string) (string, error) {
	name := agent
	if p.Name != nil {
		if n := p.Name(agent); n != "" {
			name = n
		}
	}
	dir := worktreepaths.Dir(repo)
	for {
		b := make([]byte, 3)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		path := filepath.Join(dir, name+"-"+hex.EncodeToString(b))
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
	}
}

// make adds a worktree at path, at agent's branch when it has one (git forgets a deleted folder first, so the branch is free), else at
// the base ref, and runs the project's setup in it.
func (p *Pool) make(agent, repo, base, branch, path string) (said []string, why string, err error) {
	if p.Making != nil {
		p.Making(agent)
	}
	// a lock a session that is gone left on the path keeps git's entry for it, and with it the branch, from the prune
	if _, err := worktreelock.Release(repo, path, worktreelock.Running); err != nil {
		return nil, "", err
	}
	gitrun.Run(repo, "worktree", "prune")
	ref, how := branch, "its branch"
	if branch == "" || gitrun.Run(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).RC != 0 {
		if p.Base == nil {
			return nil, "no base to make a worktree from", nil
		}
		if ref, how, err = p.Base(repo, base); err != nil || ref == "" {
			return nil, how, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, "", err
	}
	if r := gitrun.RunTimeout(repo, gitLimit, "worktree", "add", "--detach", path, ref); r.RC != 0 {
		return nil, fmt.Sprintf("could not make %s from %s: %s", path, ref, strings.TrimSpace(r.Stderr)), nil
	}
	said = append(said, fmt.Sprintf("made %s from %s (%s)", path, ref, how))
	if p.Setup != nil {
		if s := p.Setup(agent, repo, path); s != "" {
			said = append(said, s)
		}
	}
	return said, "", nil
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
		for _, b := range []string{agent, agentBranch(p, agent), hashed(agentBranch(p, agent), agent)} {
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
	branch = freeName(repo, branch, agent)
	if r := gitrun.RunTimeout(desk, gitLimit, "checkout", "-q", "-b", branch, ref); r.RC != 0 {
		return "", fmt.Sprintf("could not start %s from %s in %s: %s", branch, ref, desk, strings.TrimSpace(r.Stderr))
	}
	return branch, ""
}

// Given is what a give back did: Removed is whether the folder was deleted; Committed is the WIP commit made first, "" for none;
// Untracked is the work git cannot simply commit that kept it.
type Given struct {
	Desk, Committed string
	Removed         bool
	Untracked       []string
}

// GiveBack is the close of agent's session in its folder. The folder stays its own; with RemoveOnSleep on it is deleted, after changes to
// tracked files are committed on its branch as "WIP at close (T<task>)", never pushed (a detached HEAD gets a branch first). Untracked
// files or an operation half done keep it: they would be lost.
func (p *Pool) GiveBack(agent string, task int64) (g Given, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.giveBack(agent, task, "SELECT path, repo FROM worktree_folders WHERE holder = ?", agent)
}

// giveBack is GiveBack of the folder query finds (path, repo), under p.mu.
func (p *Pool) giveBack(agent string, task int64, query string, args ...any) (g Given, err error) {
	if !p.removeOnSleep() {
		return g, nil
	}
	var path, repo string
	switch err := p.DB.QueryRow(query, args...).Scan(&path, &repo); {
	case err == sql.ErrNoRows:
		return g, nil
	case err != nil:
		return g, err
	}
	g.Desk = path
	if !exists(path) || p.running(repo, path) {
		return g, nil
	}
	tracked, untracked, why := status(path)
	if why != "" {
		return g, fmt.Errorf("%s: %s", path, why)
	}
	if op := midOperation(path); op != "" {
		untracked = append(untracked, op) // a rebase, merge or cherry-pick half done is the agent's to finish or abort, never a WIP commit
	}
	if g.Untracked = untracked; len(untracked) > 0 {
		return g, nil
	}
	if tracked {
		if g.Committed, err = p.wip(agent, path, task); err != nil {
			return g, err
		}
	}
	if head(path) == "" && strings.TrimSpace(gitrun.Run(path, "for-each-ref", "--count=1", "--contains", "HEAD").Stdout) == "" {
		g.Untracked = []string{"commits on no branch"} // a detached HEAD no ref holds: the folder's reflog is the only trace of them
		return g, nil
	}
	if b := head(path); b != "" {
		if _, err := p.DB.Exec("UPDATE agent_state SET branch = ? WHERE agent = ?", b, agent); err != nil {
			return g, err
		}
	}
	// a lock left by the session that closed refuses the remove
	if _, err := worktreelock.Release(repo, path, worktreelock.Running); err != nil {
		return g, err
	}
	if r := gitrun.RunTimeout(repo, gitLimit, "worktree", "remove", "--force", path); r.RC != 0 {
		return g, fmt.Errorf("%s: git worktree remove: %s", path, strings.TrimSpace(r.Stderr))
	}
	g.Removed = true
	return g, nil
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
		branch = freeName(desk, branch, agent)
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

// freeName is branch when git can make it in repo, else one it can: a ref named like a folder of it (a branch "team" in the way of
// "team/agent") or under it blocks the name, so the slashes become dashes and a short hash of agent keeps it the agent's own.
func freeName(repo, branch, agent string) string {
	if !blocked(repo, branch) {
		return branch
	}
	return hashed(branch, agent)
}

// hashed is branch with its slashes as dashes and a short hash of agent after it.
func hashed(branch, agent string) string {
	sum := sha1.Sum([]byte(agent))
	return strings.ReplaceAll(branch, "/", "-") + "-" + hex.EncodeToString(sum[:3])
}

// blocked is whether a ref in repo keeps git from making branch: one named like a folder above it, or a folder of refs at its name.
func blocked(repo, branch string) bool {
	for i, c := range branch {
		if c == '/' && gitrun.Run(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch[:i]).RC == 0 {
			return true
		}
	}
	return strings.TrimSpace(gitrun.Run(repo, "for-each-ref", "--count=1", "--format=%(refname)", "refs/heads/"+branch+"/").Stdout) != ""
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

// forgetGone takes out of the pool every folder of repo that no agent holds and is gone, and has git forget it. A held one stays: its
// agent's wake makes it again at the same path.
func (p *Pool) forgetGone(repo string) error {
	folders, err := List(p.DB, repo)
	if err != nil {
		return err
	}
	pruned := false
	for _, f := range folders {
		if f.Holder != "" || exists(f.Path) {
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

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// GraceS is how long after a take a closed agent keeps its desk: a wake takes the desk before its session starts, and the sweep must not
// give it back in between.
const GraceS = 600

// Sweep gives back the folder of every agent whose session is closed (GiveBack), once its process has let go of it (Running) and GraceS
// has passed since the take. Nothing is done with RemoveOnSleep off.
func (p *Pool) Sweep(now int64) error {
	if !p.removeOnSleep() {
		return nil
	}
	rows, err := p.DB.Query("SELECT f.path, f.holder, COALESCE(s.current_task, 0) FROM worktree_folders f JOIN agent_state s ON s.agent = f.holder "+
		"WHERE COALESCE(s.closed_at, 0) != 0 AND f.last_used < ?", now-GraceS)
	if err != nil {
		return err
	}
	type held struct {
		desk, agent string
		task        int64
	}
	var all []held
	for rows.Next() {
		var h held
		if err := rows.Scan(&h.desk, &h.agent, &h.task); err != nil {
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
		if !exists(h.desk) {
			continue
		}
		p.mu.Lock()
		// read again under the lock: a wake may have taken the folder (last_used) or started its session since the list above
		_, err := p.giveBack(h.agent, h.task, "SELECT f.path, f.repo FROM worktree_folders f JOIN agent_state s ON s.agent = f.holder "+
			"WHERE f.path = ? AND f.holder = ? AND COALESCE(s.closed_at, 0) != 0 AND f.last_used < ?", h.desk, h.agent, now-GraceS)
		p.mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", h.agent, err))
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
