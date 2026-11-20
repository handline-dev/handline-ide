package reads

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/internal/worktreeprune"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/protocol/rpc"
)

// maxChangesBytes caps task.changes' diff text; the files past it are named in left_out.
const maxChangesBytes = 2 << 20

var hexSHA = regexp.MustCompile(`^[0-9a-f]{4,64}$`)

// changedFile is one file of a diff with its line counts; a binary file counts 0 and says so.
type changedFile struct {
	repo, path     string
	added, deleted int
	binary         bool
	section        string // its part of the unified diff
}

// taskCommit is one commit that names the task in its subject.
type taskCommit struct {
	root           string
	sha, subject   string
	parents        []string
	at             int64
	seq            int // topological place, landed first: the tie-break within one second
	onMain         bool
	agent          string
	files          []changedFile
	added, deleted int
}

// taskChanges is task.changes: the task's commits found by their subject, landed or on any local branch, and the files and diff of
// the one asked for or of them all.
func (e *Env) taskChanges(_ *rpc.Conn, params map[string]any) (any, error) {
	tid, err := optInt(params, "task")
	if err != nil || tid == nil {
		return nil, badParam("task is a task number", "task")
	}
	want, err := optStr(params, "commit")
	if err != nil || (want != nil && !hexSHA.MatchString(*want)) {
		return nil, badParam("commit is a commit's hex id", "commit")
	}
	t, err := queryOne(e.DB, "SELECT owner FROM tasks WHERE id = ?", *tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, noTask(*tid)
	}
	kn, err := e.knownWorktrees()
	if err != nil {
		return nil, err
	}
	var commits []*taskCommit
	for _, root := range e.taskRepos(t.str("owner")) {
		cs, err := commitsNaming(root, *tid, kn)
		if err != nil {
			return nil, err
		}
		commits = append(commits, cs...)
	}
	slices.SortFunc(commits, func(a, b *taskCommit) int { return cmp.Or(cmp.Compare(a.at, b.at), cmp.Compare(a.seq, b.seq)) })

	var asked any
	shown := commits
	if want != nil {
		shown = nil
		for _, c := range commits {
			if strings.HasPrefix(c.sha, *want) {
				shown = append(shown, c)
			}
		}
		if len(shown) != 1 {
			nf := rpc.NotFoundError(fmt.Sprintf("no commit %s of T%d", *want, *tid))
			nf.Data = map[string]any{"what": "commit", "commit": *want}
			return nil, nf
		}
		asked = shown[0].sha
	}
	files, err := filesOf(shown)
	if err != nil {
		return nil, err
	}

	outCommits := make([]any, len(commits))
	for i, c := range commits {
		var agent any
		if c.agent != "" {
			agent = c.agent
		}
		outCommits[i] = map[string]any{"sha": c.sha, "repo": filepath.Base(c.root), "subject": c.subject, "agent": agent, "at": ts(c.at),
			"on_main": c.onMain, "files": len(c.files), "added": c.added, "deleted": c.deleted}
	}
	var diff strings.Builder
	outFiles, leftOut := []any{}, []any{}
	for _, f := range files {
		outFiles = append(outFiles, map[string]any{"path": f.path, "repo": f.repo, "added": f.added, "deleted": f.deleted, "binary": f.binary})
		if diff.Len()+len(f.section) > maxChangesBytes {
			leftOut = append(leftOut, f.path)
			continue
		}
		diff.WriteString(f.section)
	}
	return map[string]any{"task": *tid, "commits": outCommits, "commit": asked, "files": outFiles, "diff": diff.String(),
		"truncated": len(leftOut) > 0, "left_out": leftOut}, nil
}

// taskRepos are the repos a task's commits are looked for in: its owner's workspace folder, or every agent's when the owner is no agent.
func (e *Env) taskRepos(owner string) []string {
	if r := e.repoOf(owner); r != "" && isDir(r) {
		return []string{r}
	}
	return worktreeprune.Repos(e.Home)
}

// changesBase is the ref landed commits are on: origin/<default branch>, else the local default branch; "" when there is neither.
func changesBase(root string) string {
	def := gitrun.DefaultBranch(root)
	for _, r := range []string{"origin/" + def, def} {
		if ok, _ := git(root, "rev-parse", "--verify", "--quiet", r+"^{commit}"); ok {
			return r
		}
	}
	return ""
}

// commitsNaming are root's commits whose subject names tid, one per change: a copy on the base first, else the newest.
func commitsNaming(root string, tid int64, kn known) ([]*taskCommit, error) {
	base := changesBase(root)
	if base == "" {
		return nil, nil
	}
	grep := []string{"log", "--no-merges", "--topo-order", "--reverse", "-F", "--grep=T" + strconv.FormatInt(tid, 10), "--format=%H%x1f%P%x1f%ct%x1f%s"}
	var all []*taskCommit
	for _, side := range []struct {
		onMain bool
		revs   []string
	}{{true, []string{base}}, {false, []string{"--branches", "--not", base}}} {
		ok, out := git(root, append(grep, side.revs...)...)
		if !ok {
			return nil, fmt.Errorf("git log in %s failed", root)
		}
		for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			f := strings.SplitN(line, "\x1f", 4)
			if len(f) != 4 || !gitrun.Names(f[3], tid) {
				continue
			}
			at, _ := strconv.ParseInt(f[2], 10, 64)
			all = append(all, &taskCommit{root: root, sha: f[0], parents: strings.Fields(f[1]), at: at, seq: len(all), subject: f[3], onMain: side.onMain})
		}
	}
	if len(all) == 0 {
		return nil, nil
	}
	ids, err := patchIDs(root, all)
	if err != nil {
		return nil, err
	}
	kept := map[string]*taskCommit{}
	var out []*taskCommit
	for _, c := range all {
		id := ids[c.sha]
		if id == "" { // an empty commit has no patch-id: it is its own change
			out = append(out, c)
			continue
		}
		if k := kept[id]; k == nil || (!k.onMain && c.at > k.at) { // the landed come first: a landed copy stays
			kept[id] = c
		}
	}
	for _, c := range kept {
		out = append(out, c)
	}
	if err := numstats(root, out); err != nil {
		return nil, err
	}
	agentsOf(root, out, kn)
	return out, nil
}

// patchIDs is each commit's `git patch-id --stable`, the same for a change and its rebased copy.
func patchIDs(root string, cs []*taskCommit) (map[string]string, error) {
	args := []string{"--no-optional-locks", "show", "--no-color", "--no-ext-diff", "--no-textconv", "--format=commit %H"}
	for _, c := range cs {
		args = append(args, c.sha)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	show := exec.CommandContext(ctx, "git", args...)
	show.Dir = root
	patch := exec.CommandContext(ctx, "git", "patch-id", "--stable")
	patch.Dir = root
	pipe, err := show.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	patch.Stdin, patch.Stdout = pipe, &out
	if err := show.Start(); err != nil {
		return nil, err
	}
	err = patch.Run()
	_ = pipe.Close() // so a show still writing gets EPIPE rather than waiting for the timeout
	if err != nil {
		_ = show.Wait()
		return nil, fmt.Errorf("git patch-id in %s: %w", root, err)
	}
	if err := show.Wait(); err != nil {
		return nil, fmt.Errorf("git show in %s: %w", root, err)
	}
	ids := map[string]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		if id, sha, ok := strings.Cut(line, " "); ok {
			ids[sha] = id
		}
	}
	return ids, nil
}

// numstats fills each commit's files and line counts, without their patches.
func numstats(root string, cs []*taskCommit) error {
	args := []string{"-c", "core.quotePath=false", "show", "--numstat", "--no-renames", "--format=%x01%H"}
	byID := map[string]*taskCommit{}
	for _, c := range cs {
		args, byID[c.sha] = append(args, c.sha), c
	}
	ok, out := git(root, args...)
	if !ok {
		return fmt.Errorf("git show --numstat in %s failed", root)
	}
	for _, part := range strings.Split(out, "\x01")[1:] {
		sha, stat, _ := strings.Cut(part, "\n")
		if c := byID[strings.TrimSpace(sha)]; c != nil {
			c.files = numstatFiles(root, stat)
			for _, f := range c.files {
				c.added += f.added
				c.deleted += f.deleted
			}
		}
	}
	return nil
}

// numstatFiles are the files of `--numstat` lines; a binary file counts 0 and says so.
func numstatFiles(root, stat string) []changedFile {
	var files []changedFile
	for _, line := range strings.Split(stat, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			continue
		}
		cf := changedFile{repo: filepath.Base(root), path: f[2], binary: f[0] == "-"}
		cf.added, _ = strconv.Atoi(f[0])
		cf.deleted, _ = strconv.Atoi(f[1])
		files = append(files, cf)
	}
	return files
}

// diffFiles runs a git diff or show with its numstat and patch and splits the patch per file, in numstat's order.
func diffFiles(root string, args ...string) ([]changedFile, error) {
	full := []string{"-c", "core.quotePath=false", args[0], "--numstat", "-p", "--no-renames", "--no-color", "--no-ext-diff", "--no-textconv"}
	ok, out := git(root, append(full, args[1:]...)...)
	if !ok {
		return nil, fmt.Errorf("git %s in %s failed", strings.Join(args, " "), root)
	}
	stat, patch := out, ""
	if i := strings.Index("\n"+out, "\ndiff --git "); i >= 0 {
		stat, patch = out[:i], out[i:]
	}
	files := numstatFiles(root, stat)
	var sections []string
	for patch != "" {
		next := strings.Index(patch[1:], "\ndiff --git ")
		if next < 0 {
			sections = append(sections, patch)
			break
		}
		sections, patch = append(sections, patch[:next+2]), patch[next+2:]
	}
	if len(sections) != len(files) {
		return nil, fmt.Errorf("git %s in %s: %d files but %d diff sections", strings.Join(args, " "), root, len(files), len(sections))
	}
	for i := range files {
		files[i].section = sections[i]
	}
	return files, nil
}

// filesOf is the files of the commits shown and their diff: one range for a run of commits that follow each other in one repo, else
// each file's sections from each commit in commit order, the files in path order.
func filesOf(cs []*taskCommit) ([]changedFile, error) {
	if len(cs) == 0 {
		return nil, nil
	}
	if contiguous(cs) {
		from := ""
		if p := cs[0].parents; len(p) == 1 {
			from = p[0]
		} else if ok, out := git(cs[0].root, "hash-object", "-t", "tree", "/dev/null"); ok { // a root commit: against the empty tree
			from = strings.TrimSpace(out)
		}
		return diffFiles(cs[0].root, "diff", from, cs[len(cs)-1].sha)
	}
	byKey := map[string]*changedFile{}
	var keys []string
	for _, c := range cs {
		files, err := diffFiles(c.root, "show", "--format=", c.sha)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			k := f.repo + "\x00" + f.path
			if have := byKey[k]; have != nil {
				have.added += f.added
				have.deleted += f.deleted
				have.binary = have.binary || f.binary
				have.section += f.section
				continue
			}
			cp := f
			byKey[k] = &cp
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]changedFile, len(keys))
	for i, k := range keys {
		out[i] = *byKey[k]
	}
	return out, nil
}

// contiguous is whether the commits are one chain in one repo, each the only parent of the next.
func contiguous(cs []*taskCommit) bool {
	for i, c := range cs {
		if len(c.parents) > 1 || c.root != cs[0].root {
			return false
		}
		if i > 0 && (len(c.parents) != 1 || c.parents[0] != cs[i-1].sha) {
			return false
		}
	}
	return true
}

// agentsOf names the agent of each commit not on the base: the one whose worktree has a branch that holds it checked out.
func agentsOf(root string, cs []*taskCommit, kn known) {
	byBranch := map[string]string{}
	for _, wt := range worktreeprune.List(root) {
		if who := kn.who[realpath(wt.Path)]; who != "" && wt.Branch != "" {
			byBranch[strings.TrimPrefix(wt.Branch, "refs/heads/")] = who
		}
	}
	if len(byBranch) == 0 {
		return
	}
	for _, c := range cs {
		if c.onMain {
			continue
		}
		_, out := git(root, "for-each-ref", "--contains", c.sha, "--format=%(refname:short)", "refs/heads")
		for _, b := range strings.Fields(out) {
			if who := byBranch[b]; who != "" {
				c.agent = who
				break
			}
		}
	}
}
