package worktreepool

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/gitrun"
)

// Removable are repo's desks ("" for every repo's) that nobody holds, none is pinned to, and that were last used before before, the one
// used longest ago first.
func Removable(q db.Q, repo string, before int64) ([]Folder, error) {
	all, err := List(q, repo)
	if err != nil {
		return nil, err
	}
	var out []Folder
	for _, f := range all {
		if f.Holder == "" && f.PinnedTo == "" && f.LastUsed < before {
			out = append(out, f)
		}
	}
	return out, nil
}

// WhyKept is what keeps a free desk from being removed, "" when nothing does: work git would lose, that is changes, untracked files, an
// operation half done, or commits on a detached HEAD that no ref holds. Ignored files (dependencies, build output) go with the desk: a new
// desk's setup makes them again.
func WhyKept(path string) string {
	if !exists(path) {
		return "" // a folder already gone holds nothing
	}
	tracked, untracked, bad := status(path)
	switch {
	case bad != "":
		return bad
	case tracked || len(untracked) > 0:
		return "it holds changes"
	case midOperation(path) != "":
		return midOperation(path)
	}
	if head(path) == "" { // a free desk is detached: its HEAD must be in some branch, tag or remote ref, or removing it orphans commits
		if r := gitrun.Run(path, "for-each-ref", "--contains", "HEAD", "--count=1", "--format=x", "refs/"); r.RC != 0 || strings.TrimSpace(r.Stdout) == "" {
			return "its HEAD is on no branch"
		}
	}
	return ""
}

// Removing pins a desk while Remove deletes its folder: no take gets it, nor treats its folder as one outside the pool.
const Removing = "removing"

// Remove deletes a free desk's folder and takes it out of the pool; the branches it had checked out stay. why says what keeps it:
// taken, pinned or used since before, a session in it, or WhyKept. The checks are made, and the desk pinned to Removing, under the
// pool's lock; git deletes the folder after, refusing one that changed in between, and then the pin is let go again.
func (p *Pool) Remove(path string, before int64) (why string, err error) {
	p.mu.Lock()
	var repo string
	var holder, pin sql.NullString
	var used int64
	err = p.DB.QueryRow("SELECT repo, holder, pinned_to, last_used FROM worktree_folders WHERE path = ?", path).Scan(&repo, &holder, &pin, &used)
	switch {
	case err == sql.ErrNoRows:
		why, err = "no longer a desk", nil
	case err != nil:
	case holder.String != "":
		why = "taken by " + holder.String
	case pin.String != "":
		why = "pinned"
	case used >= before:
		why = "used since"
	case exists(path) && p.running(repo, path):
		why = "a session runs in it"
	default:
		why = WhyKept(path)
	}
	if err == nil && why == "" {
		_, err = p.DB.Exec("UPDATE worktree_folders SET pinned_to = ? WHERE path = ?", Removing, path)
	}
	p.mu.Unlock()
	if err != nil || why != "" {
		return why, err
	}
	if !exists(path) {
		gitrun.Run(repo, "worktree", "prune")
	} else if r := gitrun.RunTimeout(repo, gitLimit, "worktree", "remove", path); r.RC != 0 {
		_, err = p.DB.Exec("UPDATE worktree_folders SET pinned_to = NULL WHERE path = ? AND pinned_to = ?", path, Removing)
		return "git: " + strings.TrimSpace(r.Stderr), err
	}
	_, err = p.DB.Exec("DELETE FROM worktree_folders WHERE path = ?", path)
	return "", err
}

// Pruned is one desk the prune removed and why.
type Pruned struct{ Path, Why string }

// Prune removes repo's free, unpinned desks beyond keep that have gone unused since before, the one used longest ago first; a desk Remove
// keeps is passed over for the next. keep counts every desk, held and pinned ones too.
func (p *Pool) Prune(repo string, keep int, before int64) ([]Pruned, error) {
	all, err := List(p.DB, repo)
	if err != nil {
		return nil, err
	}
	over := len(all) - keep
	if over <= 0 {
		return nil, nil
	}
	free, err := Removable(p.DB, repo, before)
	if err != nil {
		return nil, err
	}
	var out []Pruned
	for _, f := range free {
		if len(out) == over {
			break
		}
		why, err := p.Remove(f.Path, before)
		if err != nil {
			return out, err
		}
		if why == "" {
			out = append(out, Pruned{f.Path, fmt.Sprintf("over the %d kept, unused %dd", keep, (p.now()-f.LastUsed)/86400)})
		}
	}
	return out, nil
}

// The founder's machine targets in config.yaml, in GB; 0, the default, is off. Under DiskKeepFreeKey of free disk the free desks go,
// oldest first; over AgentsMemoryMaxKey of the agents' memory the longest-idle sessions close.
const (
	DiskKeepFreeKey    = "disk_keep_free_gb"
	AgentsMemoryMaxKey = "agents_memory_max_gb"
)

// GB is one of the targets in bytes: 0 when it is off, unset or not a whole number above 0.
func GB(cfg map[string]any, key string) int64 {
	switch n := cfg[key].(type) {
	case int:
		return max(int64(n), 0) << 30
	case int64:
		return max(n, 0) << 30
	}
	return 0
}

// Free removes free, unpinned desks of every project, the one used longest ago first, while short says the disk is still short and
// until the measured size of the desks removed adds up to need (a desk not yet measured counts as all of it): the space a removal frees
// may show late (a snapshot), and that must not take every desk. A desk Remove keeps is passed over.
func (p *Pool) Free(short func() bool, need int64, size func(path string) int64) ([]Pruned, error) {
	before := p.now() + 1
	free, err := Removable(p.DB, "", before)
	if err != nil {
		return nil, err
	}
	var out []Pruned
	var freed int64
	for _, f := range free {
		if !short() || freed >= need {
			break
		}
		n := size(f.Path)
		if n <= 0 {
			n = need
		}
		why, err := p.Remove(f.Path, before)
		if err != nil {
			return out, err
		}
		if why == "" {
			freed += n
			out = append(out, Pruned{f.Path, "free disk under its target"})
		}
	}
	return out, nil
}
