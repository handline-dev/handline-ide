package reads

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/handline/handline/go/internal/worktreeprune"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

const (
	recentMax    = 12
	recentBudget = 300 * time.Millisecond
	recentDepth  = 3 // ~/code/org/repo
)

// recentNever are the home folders a scan never enters: macOS asks the person for the first four, the rest hold media and apps.
var recentNever = []string{"Documents", "Desktop", "Downloads", "Library", "Pictures", "Music", "Movies", "Applications"}

// recentBundles are folder name endings never entered: app bundles and media libraries.
var recentBundles = []string{".app", ".photoslibrary", ".musiclibrary"}

type recentDir struct {
	path  string
	depth int
}

type recentFolder struct {
	path, name string
	touched    time.Time
}

func (e *Env) reposRecent(_ *rpc.Conn, _ map[string]any) (any, error) {
	deadline := time.Now().Add(recentBudget)
	known := map[string]bool{}
	for _, p := range worktreeprune.Repos(e.Home) {
		known[realpath(p)] = true
	}
	out := []any{}
	for _, f := range recentRepos(realpath(paths.UserHome()), known, deadline) {
		out = append(out, map[string]any{"path": f.path, "name": f.name, "touched_at": ts(f.touched.Unix())})
	}
	return map[string]any{"folders": out}, nil
}

// recentRepos walks home breadth first to recentDepth levels, by stats alone, until the deadline, for the newest recentMax repos not in known.
func recentRepos(home string, known map[string]bool, deadline time.Time) []recentFolder {
	st, err := os.Stat(home)
	if err != nil {
		return nil
	}
	dev := devOf(st)
	never := map[string]bool{}
	for _, n := range recentNever {
		never[filepath.Join(home, n)] = true
	}
	var found []recentFolder
	queue := []recentDir{{home, 0}}
	for len(queue) > 0 && time.Now().Before(deadline) {
		dir := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(dir.path)
		if err != nil {
			continue
		}
		for _, d := range entries {
			if !time.Now().Before(deadline) {
				break
			}
			if !d.IsDir() || strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || isBundle(d.Name()) {
				continue
			}
			p := filepath.Join(dir.path, d.Name())
			if never[p] || known[p] {
				continue
			}
			info, err := d.Info()
			if err != nil || devOf(info) != dev {
				continue
			}
			switch touched, git := gitTouched(p); git {
			case gitRepo:
				found = append(found, recentFolder{path: p, name: d.Name(), touched: touched})
			case gitNone:
				if dir.depth+1 < recentDepth {
					queue = append(queue, recentDir{p, dir.depth + 1})
				}
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].touched.After(found[j].touched) })
	if len(found) > recentMax {
		found = found[:recentMax]
	}
	return found
}

func isBundle(name string) bool {
	for _, ext := range recentBundles {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

type gitKind int

const (
	gitNone  gitKind = iota // no .git: a folder to look inside
	gitOther                // a .git that is no repo of its own, such as a linked worktree's file: neither listed nor entered
	gitRepo
)

// gitTouched is what dir's .git is and, for a repo, the later mtime of its HEAD and index.
func gitTouched(dir string) (time.Time, gitKind) {
	g := filepath.Join(dir, ".git")
	st, err := os.Lstat(g)
	if err != nil {
		return time.Time{}, gitNone
	}
	if !st.IsDir() {
		return time.Time{}, gitOther
	}
	head, err := os.Lstat(filepath.Join(g, "HEAD"))
	if err != nil {
		return time.Time{}, gitOther
	}
	t := head.ModTime()
	if idx, err := os.Lstat(filepath.Join(g, "index")); err == nil && idx.ModTime().After(t) {
		t = idx.ModTime()
	}
	return t, gitRepo
}

func devOf(fi os.FileInfo) uint64 {
	if s, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Dev) //nolint:unconvert // Dev is int32 on darwin, uint64 on linux
	}
	return 0
}
