package filerw

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/protocol/rpc"
)

// listDefault and listMax are files.list's limit when none is given and the most it takes.
const (
	listDefault = 50
	listMax     = 200
)

// list is files.list: what git tracks plus the untracked files it does not ignore, the regular files among them, best matches first.
func (e *Env) list(_ *rpc.Conn, p map[string]any) (any, error) {
	agent, _ := p["agent"].(string)
	if agent == "" {
		return nil, bad("agent is an agent's name", "agent")
	}
	query := ""
	if v, has := p["query"]; has && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, bad("query is a string", "query")
		}
		query = strings.ToLower(s)
	}
	limit := listDefault
	if v, has := p["limit"]; has && v != nil {
		n, ok := rpc.AsInt(v)
		if !ok || n < 1 || n > listMax {
			return nil, bad(fmt.Sprintf("limit is an integer from 1 to %d", listMax), "limit")
		}
		limit = int(n)
	}
	wt, err := e.worktree(agent)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(wt)
	if err != nil {
		// deleted while its agent sleeps (worktreepool.RemoveOnSleepKey): its files are its branch's
		sizes, ok := e.branchFiles(agent, wt)
		if !ok {
			return nil, rpc.NotFoundError("the worktree of " + addr.Ref(agent) + " is gone")
		}
		return listed(sizes, query, limit, func(path string) (int64, bool) { n, ok := sizes[path]; return n, ok }), nil
	}
	defer root.Close()
	r := gitrun.Run(wt, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if r.RC != 0 {
		return nil, fmt.Errorf("git ls-files in %s: %s", wt, strings.TrimSpace(r.Stderr))
	}
	all := map[string]int64{}
	for _, rel := range strings.Split(r.Stdout, "\x00") {
		if rel != "" {
			all[rel] = 0 // a conflicted file is listed once per stage
		}
	}
	// Only the files returned, and one to tell whether more follow, are looked at on disk.
	return listed(all, query, limit, func(path string) (int64, bool) {
		fi, err := root.Lstat(path)
		if err != nil || !fi.Mode().IsRegular() { // deleted since it was tracked, a link, or a submodule
			return 0, false
		}
		return fi.Size(), true
	}), nil
}

// listed is files.list over paths: those matching query, best first, each kept while size says it is a regular file, up to limit.
func listed(paths map[string]int64, query string, limit int, size func(path string) (int64, bool)) map[string]any {
	type hit struct {
		path string
		tier int
	}
	var hits []hit
	for rel := range paths {
		if tier, ok := match(rel, query); ok {
			hits = append(hits, hit{rel, tier})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if query != "" && a.tier != b.tier {
			return a.tier < b.tier
		}
		if query != "" && len(a.path) != len(b.path) {
			return len(a.path) < len(b.path)
		}
		return a.path < b.path
	})
	files, more := []any{}, false
	for _, h := range hits {
		n, ok := size(h.path)
		if !ok {
			continue
		}
		if len(files) == limit {
			more = true
			break
		}
		files = append(files, map[string]any{"path": h.path, "size": n})
	}
	return map[string]any{"files": files, "more": more}
}

// branchFiles is each regular file on the branch of agent's worktree wt, deleted while it sleeps, by path with its size; false when its
// repository or branch is not known.
func (e *Env) branchFiles(agent, wt string) (map[string]int64, bool) {
	// the pool records the folder with its parent's links resolved (macOS's /var is /private/var); the parent stays when it is deleted
	canon := wt
	if dir, err := filepath.EvalSymlinks(filepath.Dir(wt)); err == nil {
		canon = filepath.Join(dir, filepath.Base(wt))
	}
	var repo, branch sql.NullString
	if err := e.DB.QueryRow("SELECT (SELECT repo FROM worktree_folders WHERE path IN (?, ?)), (SELECT branch FROM agent_state WHERE agent = ?)",
		wt, canon, agent).Scan(&repo, &branch); err != nil || repo.String == "" || branch.String == "" {
		return nil, false
	}
	r := gitrun.Run(repo.String, "ls-tree", "-r", "-l", "-z", "refs/heads/"+branch.String)
	if r.RC != 0 {
		return nil, false
	}
	out := map[string]int64{}
	for _, line := range strings.Split(r.Stdout, "\x00") {
		meta, path, ok := strings.Cut(line, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 4 || f[1] != "blob" || f[0] == "120000" { // a link is no file to open
			continue
		}
		n, _ := strconv.ParseInt(f[3], 10, 64)
		out[path] = n
	}
	return out, true
}

// match is whether the lowercased query q is a subsequence of path, compared lowercased, and how good the match is: 0 inside the file's
// name, 1 elsewhere in the path, 2 spread out. An empty query matches everything.
func match(path, q string) (int, bool) {
	if q == "" {
		return 0, true
	}
	lower := strings.ToLower(path)
	if strings.Contains(lower[strings.LastIndex(lower, "/")+1:], q) {
		return 0, true
	}
	if strings.Contains(lower, q) {
		return 1, true
	}
	rest := []rune(q)
	for _, c := range lower {
		if len(rest) > 0 && c == rest[0] {
			rest = rest[1:]
		}
	}
	return 2, len(rest) == 0
}
