// Package worktreeprune is the one rule for which worktrees are done with (tools/prune_worktrees.py runs it too): a worktree goes only
// when its branch is merged into the repository's default branch (gitrun.Base: origin's copy of it when the clone has one, since a
// push moves that and not the clone's own branch, else the local one), it has no uncommitted or untracked file, and no agent works in
// it. The default branch's own, a locked one and one on no branch stay. Removing one leaves its branch: `git branch -d` takes that.
//
// Origin's copy of the default branch is as the clone last heard it: a ref the agents' own fetch and push write, and behind the
// remote's when that was rewound since the last fetch. A prune and a retire both take its word, since a push is how work lands and
// nothing moves the clone's own branch: asking again at a retire would ask about every branch that landed by a push.
package worktreeprune

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/paths"
)

// Worktree is one entry of `git worktree list --porcelain`.
type Worktree struct {
	Path, Branch           string
	Bare, Prunable, Locked bool
}

// List is the worktrees of the repository at repo, its own checkout first; nil when git cannot say.
func List(repo string) []Worktree {
	r := gitrun.Run(repo, "worktree", "list", "--porcelain")
	if r.RC != 0 {
		return nil
	}
	var out []Worktree
	var cur *Worktree
	for _, line := range append(strings.Split(r.Stdout, "\n"), "") {
		if line == "" {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = nil
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		if cur == nil {
			cur = &Worktree{}
		}
		switch key {
		case "worktree":
			cur.Path = value
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			cur.Bare = true
		case "prunable":
			cur.Prunable = true
		case "locked":
			cur.Locked = true
		}
	}
	return out
}

// Real is path with its links resolved, as the paths are compared; path itself when it cannot be.
func Real(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return filepath.Clean(path)
}

// AgentWorktrees are the real paths of every worktree an agent records, but skip's, and every desk but those pinned to skip: a desk is
// the project's, kept when its agent leaves or is retired.
func AgentWorktrees(q db.Q, skip string) (map[string]bool, error) {
	out := map[string]bool{}
	err := recorded(q, "SELECT agent, COALESCE(worktree, '') FROM agent_state "+
		"UNION ALL SELECT COALESCE(pinned_to, ''), path FROM worktree_folders", func(agent, path string) {
		if agent == "" || agent != skip {
			out[path] = true
		}
	})
	return out, err
}

// Of are the real paths of the worktree agent records and the desks pinned to it.
func Of(q db.Q, agent string) ([]string, error) {
	var out []string
	err := recorded(q, "SELECT agent, COALESCE(worktree, '') FROM agent_state WHERE agent = ? "+
		"UNION ALL SELECT pinned_to, path FROM worktree_folders WHERE pinned_to = ?", func(_, path string) {
		out = append(out, path)
	}, agent, agent)
	return out, err
}

// recorded hands each worktree path the rows of query record to each, as a real path. `hl launch` records <repo id>::<path>.
func recorded(q db.Q, query string, each func(agent, path string), args ...any) error {
	rows, err := q.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var agent, p string
		if err := rows.Scan(&agent, &p); err != nil {
			return err
		}
		if p == "" {
			continue
		}
		if _, path, ok := strings.Cut(p, "::"); ok {
			p = path
		}
		each(agent, Real(p))
	}
	return rows.Err()
}

// WhyKept is why the worktree stays, "" when it may be removed. agents are the paths agents work in (AgentWorktrees), here the
// worktree this runs from ("" for none).
func WhyKept(wt Worktree, agents map[string]bool, here string) string {
	path := Real(wt.Path)
	switch {
	case wt.Bare || wt.Prunable:
		return "not a checkout"
	case here != "" && path == Real(here):
		return "this is run from it"
	case agents[path]:
		return "an agent works in it"
	case wt.Locked:
		return "locked"
	case wt.Branch == "":
		return OnNoBranch
	}
	base := gitrun.BaseOf(wt.Path)
	if wt.Branch == base.Branch {
		return base.Branch
	}
	if ok, _ := base.Reaches(wt.Path, "refs/heads/"+wt.Branch); !ok {
		return wt.Branch + notMerged + base.Branch
	}
	if st := gitrun.Run(wt.Path, "status", "--porcelain"); st.RC != 0 || strings.TrimSpace(st.Stdout) != "" {
		return uncommitted
	}
	return ""
}

// Tag marks the founder's task that asks what to do with a retired member's worktree that holds work.
const Tag = "worktree-removal"

// The answers for a worktree that holds work, as agent.retire takes them (worktree) and the founder's Tag task offers them as buttons.
const (
	Keep  = "keep"  // it stays
	Save  = "save"  // what is left goes to a rescue branch, then it is removed
	Force = "force" // it is removed with whatever it holds; its branch stays
)

// OnNoBranch is WhyKept's answer for a worktree with a detached HEAD.
const OnNoBranch = "on no branch"

// DetachedWork is WhyKept for a worktree on no branch, judged by its HEAD: "" when the default branch has HEAD and nothing is
// uncommitted.
func DetachedWork(path string) string {
	base := gitrun.BaseOf(path)
	if ok, _ := base.Reaches(path, "HEAD"); !ok {
		return "HEAD" + notMerged + base.Branch
	}
	if st := gitrun.Run(path, "status", "--porcelain"); st.RC != 0 || strings.TrimSpace(st.Stdout) != "" {
		return uncommitted
	}
	return ""
}

const (
	notMerged   = " is not merged into " // the default branch's name follows; no branch name holds a space, so these words mark the reason
	uncommitted = "uncommitted or untracked files"
)

// HoldsWork is whether why (WhyKept's) says the worktree holds work nobody else has: commits the default branch lacks, or files not
// committed.
func HoldsWork(why string) bool { return why == uncommitted || strings.Contains(why, notMerged) }

// Rescue keeps what the worktree holds, then removes it: the files not committed go in one commit on a new branch rescue/<member>-<date>
// (the branch the worktree was on stays as it is), so nothing is lost but what git ignores. It answers the branch.
func Rescue(path, member string, now time.Time) (branch string, err error) {
	branch = "rescue/" + member + "-" + now.Format("2006-01-02")
	for i := 2; gitrun.Run(path, "rev-parse", "--verify", "-q", "refs/heads/"+branch).RC == 0; i++ {
		branch = fmt.Sprintf("rescue/%s-%s-%d", member, now.Format("2006-01-02"), i)
	}
	steps := [][]string{
		{"checkout", "-q", "-b", branch},
		{"add", "-A"},
		{"-c", "user.name=handline", "-c", "user.email=handline@localhost", "commit", "-q", "--allow-empty", "-m", "rescue: what " + member + "'s worktree held when it was removed"},
	}
	for _, args := range steps {
		if r := gitrun.Run(path, args...); r.RC != 0 {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(r.Stderr))
		}
	}
	if ok, why := Remove(path); !ok {
		return branch, fmt.Errorf("the rescue branch %s is made, but the worktree stays: %s", branch, why)
	}
	return branch, nil
}

// RemoveForce removes the worktree with whatever it holds; the branch it was on stays.
func RemoveForce(path string) (ok bool, why string) {
	r := gitrun.Run(path, "worktree", "remove", "--force", path)
	if r.RC != 0 {
		return false, strings.TrimSpace(r.Stderr)
	}
	return true, ""
}

// Remove removes the worktree at path, never by force: git refuses one that became dirty since it was judged.
func Remove(path string) (ok bool, why string) {
	r := gitrun.Run(path, "worktree", "remove", path)
	if r.RC != 0 {
		return false, strings.TrimSpace(r.Stderr)
	}
	return true, ""
}

// Repos are the repositories the agents were told to work in (their workspace folders), each once, sorted.
func Repos(home string) []string {
	seen := map[string]bool{}
	var out []string
	roster := team.NewRoster(home)
	for _, name := range paths.ListAgents(home) {
		cfg, err := roster.Read(name)
		if err != nil {
			continue
		}
		if repo, _ := cfg["repo"].(string); repo != "" && !seen[repo] {
			if st, err := os.Stat(repo); err == nil && st.IsDir() {
				seen[repo] = true
				out = append(out, repo)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Unsaved is what the worktree holds that its repository's default branch does not: whether its HEAD is merged into it, its changed or
// untracked files and its commits not on it.
func Unsaved(path string) (merged bool, files, commits int) {
	merged, commits = Ahead(path)
	if r := gitrun.Run(path, "status", "--porcelain"); r.RC == 0 && strings.TrimSpace(r.Stdout) != "" {
		files = len(strings.Split(strings.TrimSpace(r.Stdout), "\n"))
	}
	return merged, files, commits
}

// Ahead is Unsaved without the files: whether the worktree's HEAD is merged into the default branch (origin's when there is one), and
// its commits not on it.
func Ahead(path string) (merged bool, commits int) { return AheadOf(path, gitrun.BaseOf(path)) }

// AheadOf is Ahead against the base its caller has already read.
func AheadOf(path string, base gitrun.Base) (merged bool, commits int) {
	merged, _ = base.Reaches(path, "HEAD")
	if revs, ok := base.NotIn("HEAD"); ok {
		if r := gitrun.Run(path, append([]string{"rev-list", "--count"}, revs...)...); r.RC == 0 {
			commits, _ = strconv.Atoi(strings.TrimSpace(r.Stdout))
		}
	}
	return merged, commits
}

// SizePrefix starts the meta name a worktree's measured size is kept under: the prefix, then its real path; the value is the bytes.
const SizePrefix = "worktree_size:"

// SizeEvery is how often the worktrees' sizes are measured, in seconds: a walk of a worktree with its dependencies takes a while.
const SizeEvery = int64(3600)

// Size is the bytes the files under path take on disk, as du counts them.
func Size(path string) (int64, error) {
	out, err := exec.Command("du", "-sk", path).Output()
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, fmt.Errorf("du said nothing for %s", path)
	}
	kb, err := strconv.ParseInt(fields[0], 10, 64)
	return kb * 1024, err
}

// DeleteMergedBranch deletes branch in repo with `git branch -d`, never -D: git keeps a branch not merged, and false says so. The
// repo's default branch is never deleted, and the branch is named after "--", so its name is never read as an option.
func DeleteMergedBranch(repo, branch string) bool {
	return branch != "" && branch != gitrun.DefaultBranch(repo) && gitrun.Run(repo, "branch", "-d", "--", branch).RC == 0
}

// Held is a worktree of an agent that holds work: its path, its changed or untracked files and its commits not on Base, its
// repository's default branch.
type Held struct {
	Path           string
	Files, Commits int
	Base           string
}

// Holding are the worktrees agent records that hold work nobody else has (HoldsWork): what a retire must ask about before it goes.
func Holding(q db.Q, agent string) ([]Held, error) {
	mine, err := Of(q, agent)
	if err != nil {
		return nil, err
	}
	others, err := AgentWorktrees(q, agent)
	if err != nil {
		return nil, err
	}
	var out []Held
	for _, path := range mine {
		wt, ok := Find(List(path), path)
		if !ok || !HoldsWork(WhyKept(wt, others, "")) {
			continue
		}
		_, files, commits := Unsaved(path)
		out = append(out, Held{Path: path, Files: files, Commits: commits, Base: gitrun.DefaultBranch(path)})
	}
	return out, nil
}

// Find is the worktree of all at path.
func Find(all []Worktree, path string) (Worktree, bool) {
	for _, wt := range all {
		if Real(wt.Path) == Real(path) {
			return wt, true
		}
	}
	return Worktree{}, false
}
