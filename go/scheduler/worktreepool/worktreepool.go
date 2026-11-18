// Package worktreepool keeps a project's desks: the worktree folders under <repo>.worktrees.noindex that agents take when a session
// starts and give back when it closes. A desk stays when it is given back, so the next agent starts in a warm folder; the work stays on
// the agent's own branch, which every desk shares through the one repository. The folders are worktree_folders' rows; the one an agent
// holds or last held is its agent_state.worktree, and its branch agent_state.branch.
package worktreepool

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/gitrun"
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
	// Base is the ref a new desk and a new branch start from (lifecycle.PickBase over the repo's default branch), and why it was chosen.
	Base func(repo string) (ref, why string, err error)
	// Branch is the name of a new branch for agent, the first time it takes a desk without one of its own.
	Branch func(agent string) string
	// Setup runs the project's setup in a desk just made, and is what to tell of it ("" for nothing). nil runs none.
	Setup func(agent, repo, desk string) string
	Now   func() time.Time

	mu sync.Mutex
}

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

// Taken is the desk a take gave: How is "held" (it held it already), "pinned", "last", "free" or "new"; Said is what to tell.
type Taken struct {
	Path, Branch, How string
	Said              []string
}

// Take gives agent a desk of repo and checks out its branch there: the desk it holds, else one pinned to it, else the one it used last
// when free, else the free desk used longest ago, else a new desk-<n> with the lowest free number. A free desk with changes in it is
// passed over. why is set, and the desk not taken, when the branch cannot be checked out (git says why).
func (p *Pool) Take(agent, repo string) (t Taken, why string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	repo = canonical(repo)
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
			if ok(f) && exists(f.Path) && (f.Holder == agent || clean(f.Path)) {
				t.Path, t.How = f.Path, how
				return true
			}
		}
		return false
	}
	free := func(f Folder) bool { return f.Holder == "" && (f.PinnedTo == "" || f.PinnedTo == agent) }
	switch {
	case pick("held", func(f Folder) bool { return f.Holder == agent }):
	case pick("pinned", func(f Folder) bool { return f.Holder == "" && f.PinnedTo == agent }):
	case last != "" && pick("last", func(f Folder) bool { return free(f) && samePath(f.Path, last) }):
	case pick("free", free): // List is oldest first
	default:
		desk, said, why, err := p.makeDesk(agent, repo, folders)
		if err != nil || why != "" {
			return t, why, err
		}
		t.Path, t.How, t.Said = desk, "new", said
	}
	if t.How == "held" {
		t.Branch = head(t.Path) // its own desk, as it left it
	} else if t.Branch, why = p.checkout(agent, repo, t.Path, branch); why != "" {
		return Taken{}, why, nil
	}
	if _, err := p.DB.Exec("UPDATE worktree_folders SET holder = ?, untracked_told = 0, last_agent = ?, last_used = ? WHERE path = ?",
		agent, agent, p.now(), t.Path); err != nil {
		return t, "", err
	}
	return t, "", p.setAgent(agent, t.Path, t.Branch)
}

// makeDesk adds desk-<n>, n the lowest number no folder of repo has, at the base ref, and runs the project's setup in it.
func (p *Pool) makeDesk(agent, repo string, folders []Folder) (desk string, said []string, why string, err error) {
	if p.Base == nil {
		return "", nil, "no base to make a desk from", nil
	}
	ref, how, err := p.Base(repo)
	if err != nil || ref == "" {
		return "", nil, how, err
	}
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
		desk = filepath.Join(dir, name)
		break
	}
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

// checkout puts agent's branch in desk: the one it has, else a new one from the base. A branch checked out in another desk is not
// forced: git refuses, and that is the why.
func (p *Pool) checkout(agent, repo, desk, branch string) (string, string) {
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
		branch = agent
		if p.Branch != nil {
			branch = p.Branch(agent)
		}
	}
	if p.Base == nil {
		return "", "no base to start " + branch + " from"
	}
	ref, how, err := p.Base(repo)
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
	var path string
	var told int
	switch err := p.DB.QueryRow("SELECT path, untracked_told FROM worktree_folders WHERE holder = ?", agent).Scan(&path, &told); {
	case err == sql.ErrNoRows:
		return g, nil
	case err != nil:
		return g, err
	}
	g.Desk = path
	if exists(path) {
		tracked, untracked, why := status(path)
		if why != "" {
			return g, fmt.Errorf("%s: %s", path, why)
		}
		if len(untracked) > 0 {
			g.Untracked = untracked
			if told == 0 {
				g.Tell = fmt.Sprintf("Your desk %s has files git does not track (%s); it stays yours until you commit or delete them.",
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
// records as its worktree is held by that agent, and its branch becomes the agent's.
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
		holder := owners[path]
		res, err := p.DB.Exec("INSERT INTO worktree_folders(path, repo, holder, last_agent, last_used) VALUES(?,?,?,?,?) ON CONFLICT(path) DO NOTHING",
			path, repo, nullStr(holder), nullStr(holder), p.now())
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

// recorded is each agent's worktree, by its real path.
func (p *Pool) recorded() (map[string]string, error) {
	rows, err := p.DB.Query("SELECT agent, worktree FROM agent_state WHERE worktree IS NOT NULL AND worktree != ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var agent, wt string
		if err := rows.Scan(&agent, &wt); err != nil {
			return nil, err
		}
		if path := worktreepaths.RecordedPath(wt); path != "" {
			out[canonical(path)] = agent
		}
	}
	return out, rows.Err()
}

// List is repo's desks, the one used longest ago first.
func List(q db.Q, repo string) ([]Folder, error) {
	rows, err := q.Query("SELECT path, repo, COALESCE(holder, ''), COALESCE(last_agent, ''), COALESCE(pinned_to, ''), last_used "+
		"FROM worktree_folders WHERE repo = ? ORDER BY last_used, path", canonical(repo))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Folder
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.Path, &f.Repo, &f.Holder, &f.LastAgent, &f.PinnedTo, &f.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
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
