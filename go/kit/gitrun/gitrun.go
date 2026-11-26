// Package gitrun runs git and hands back what it said, the same way for every caller (handline/infra/gitrun.py).
package gitrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// IntegrationBranch is the default branch of a repository made here, and the guess for one that names none. The branch a repository's
// work is branched from and merged into is asked of it: DefaultBranch.
const IntegrationBranch = "main"

// Result is what git said, untouched. RC is -1 when git could not be run at all (missing, or past the timeout) and Stderr says why.
type Result struct {
	RC             int
	Stdout, Stderr string
}

// Run runs git in cwd with a 15 second limit.
func Run(cwd string, args ...string) Result { return RunTimeout(cwd, 15*time.Second, args...) }

func RunTimeout(cwd string, limit time.Duration, args ...string) Result {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return Result{0, out.String(), errb.String()}
	case errors.As(err, &exit) && ctx.Err() == nil:
		return Result{exit.ExitCode(), out.String(), errb.String()}
	case ctx.Err() != nil:
		return Result{-1, "", "git timed out"}
	}
	return Result{-1, "", err.Error()}
}

// NoPrompts is the environment of a git run with no one at it: neither git nor ssh asks for anything, so what needs a password or a
// new host key fails at once; and git uses no transport but protocols, the submodules' and redirects' too.
func NoPrompts(protocols []string) []string {
	env := []string{"GIT_ALLOW_PROTOCOL=" + strings.Join(protocols, ":"), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_ASKPASS=", "SSH_ASKPASS="}
	if os.Getenv("GIT_SSH_COMMAND") == "" && Run("", "config", "--get", "core.sshCommand").RC != 0 {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}

// DefaultBranch is the branch the repository at repo merges into: origin's HEAD when it names a branch origin has, else main or master
// when it has one, else the branch its main worktree has checked out, else IntegrationBranch. repo may be any of its worktrees.
func DefaultBranch(repo string) string {
	if b := originHead(repo); b != "" {
		return b
	}
	for _, b := range []string{IntegrationBranch, "master"} {
		for _, ref := range []string{"refs/heads/", "refs/remotes/origin/"} {
			if Run(repo, "rev-parse", "-q", "--verify", ref+b).RC == 0 {
				return b
			}
		}
	}
	// the main worktree is listed first; a linked worktree's own HEAD is the work, not what it merges into
	if r := Run(repo, "worktree", "list", "--porcelain"); r.RC == 0 {
		first, _, _ := strings.Cut(r.Stdout, "\n\n")
		for _, l := range strings.Split(first, "\n") {
			if b, ok := strings.CutPrefix(l, "branch refs/heads/"); ok {
				return b
			}
		}
	}
	return IntegrationBranch
}

// MainRef is the ref that says what main holds in repo: origin's main when repo has one, since a push moves it and nothing but a
// fetch into it moves the clone's own main; else the local main, as in a repo with no remote.
func MainRef(repo string) string {
	if origin := "refs/remotes/origin/" + IntegrationBranch; Run(repo, "rev-parse", "-q", "--verify", origin).RC == 0 {
		return origin
	}
	return "refs/heads/" + IntegrationBranch
}

// OnMain is git merge-base --is-ancestor rev MainRef(repo): RC 0 when main reaches rev, 1 when it does not, else git's failure.
func OnMain(repo, rev string) Result {
	return Run(repo, "merge-base", "--is-ancestor", rev, MainRef(repo))
}

// originHead is the branch origin/HEAD names, "" when it names none, or one origin no longer has: the remote dropped or renamed it and a
// prune took its ref, while git keeps the name.
func originHead(repo string) string {
	r := Run(repo, "symbolic-ref", "-q", "refs/remotes/origin/HEAD")
	b, ok := strings.CutPrefix(strings.TrimSpace(r.Stdout), "refs/remotes/origin/")
	if r.RC != 0 || !ok || b == "" || Run(repo, "rev-parse", "-q", "--verify", "refs/remotes/origin/"+b+"^{commit}").RC != 0 {
		return ""
	}
	return b
}

// NameOriginHead gives origin/HEAD a branch when it names none that origin has: the repository's default branch (DefaultBranch), which
// a session then reads from it as the branch work merges into. One that names a branch of origin is left as it is, whichever branch
// that is: it is the remote's own word, or the person's. True when it was set; false when it was left, or origin has no such branch.
func NameOriginHead(cwd string) bool {
	if originHead(cwd) != "" {
		return false
	}
	return Run(cwd, "remote", "set-head", "origin", DefaultBranch(cwd)).RC == 0
}

// Base is where a repository's landed work is. Branch is its default branch, and Refs are the refs of it the repository has: origin's
// copy first ("origin/<branch>"), then the local branch. What has landed is what the first of them reaches (Ref): origin's copy when
// the clone has one, since a push moves it and nothing but a fetch into it moves the clone's own branch, so a commit on the local
// branch alone has not landed; else the local branch, as in a repository with no remote. This is MainRef and OnMain for a default
// branch of any name.
type Base struct {
	Branch string
	Refs   []string
}

// BaseOf is the Base of the repository at repo, which may be any of its worktrees.
func BaseOf(repo string) Base {
	b := Base{Branch: DefaultBranch(repo)}
	for _, ref := range []string{"origin/" + b.Branch, b.Branch} {
		if Run(repo, "rev-parse", "--verify", "--quiet", b.full(ref)+"^{commit}").RC == 0 {
			b.Refs = append(b.Refs, ref)
		}
	}
	return b
}

// full is one of Refs by its whole name, which no tag or other ref of the same short name can stand in for.
func (b Base) full(ref string) string {
	if ref == b.Branch {
		return "refs/heads/" + ref
	}
	return "refs/remotes/" + ref
}

// Ref is the ref landed work is measured against: origin's copy of the default branch, else the local one; "" when there is neither.
func (b Base) Ref() string {
	if len(b.Refs) == 0 {
		return ""
	}
	return b.Refs[0]
}

// Reaches is whether rev has landed: Ref has it as an ancestor, or is it. why is what git said when it does not and git said
// anything; a rev that is simply not merged leaves it "".
func (b Base) Reaches(repo, rev string) (ok bool, why string) {
	if len(b.Refs) == 0 {
		return false, "this repo has no " + b.Branch + " branch"
	}
	r := Run(repo, "merge-base", "--is-ancestor", rev, b.full(b.Refs[0]))
	if r.RC == 0 {
		return true, ""
	}
	return false, strings.TrimSpace(r.Stderr + r.Stdout)
}

// NotIn are the commits of tip that have not landed, as the revisions to hand git log or rev-list: tip, less what Ref has. ok is
// false when the base has no ref, and so nothing to measure tip against.
func (b Base) NotIn(tip string) (revs []string, ok bool) {
	if len(b.Refs) == 0 {
		return nil, false
	}
	return []string{tip, "--not", b.full(b.Refs[0])}, true
}

// HasOrigin is whether the repository at repo names a remote origin: only then can a commit have landed there and not be known here
// yet. Nothing in this package fetches: origin's copy of a branch is as the clone last heard it, from its own fetch or push.
func HasOrigin(repo string) bool {
	return Run(repo, "config", "--get", "remote.origin.url").RC == 0
}

// Names is whether subject's last parenthesis names task tid as a whole token: "(T9201)" or "(T9139, T9140)".
func Names(subject string, tid int64) bool {
	open := strings.LastIndex(subject, "(")
	if open < 0 {
		return false
	}
	end := strings.Index(subject[open:], ")")
	if end < 0 {
		return false
	}
	want := "T" + strconv.FormatInt(tid, 10)
	for _, part := range strings.Split(subject[open+1:open+end], ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

// TaskFile is one file the commits naming a task changed, and whether each of them that touched it deleted it.
type TaskFile struct {
	Path    string
	Deleted bool
}

// TaskFiles are the files changed by repo's commits that name task tid (Names), landed on its default branch or on any local branch,
// sorted by path.
func TaskFiles(repo string, tid int64) ([]TaskFile, error) {
	revs := []string{"--branches"}
	if b := BaseOf(repo); b.Ref() != "" && b.Ref() != b.Branch {
		revs = append(revs, b.Ref())
	}
	r := RunTimeout(repo, 20*time.Second, append([]string{"--no-optional-locks", "-c", "core.quotePath=false", "log", "--no-merges", "-F", "--grep=T" + strconv.FormatInt(tid, 10),
		"--format=%x1e%s", "--name-status", "--no-renames"}, revs...)...)
	if r.RC != 0 {
		return nil, fmt.Errorf("git log in %s: %s", repo, strings.TrimSpace(r.Stderr))
	}
	deleted := map[string]bool{}
	for _, c := range strings.Split(r.Stdout, "\x1e")[1:] {
		subject, files, _ := strings.Cut(c, "\n")
		if !Names(subject, tid) {
			continue
		}
		for _, l := range strings.Split(files, "\n") {
			status, p, ok := strings.Cut(l, "\t")
			if !ok {
				continue
			}
			d, seen := deleted[p]
			deleted[p] = (d || !seen) && status == "D"
		}
	}
	out := make([]TaskFile, 0, len(deleted))
	for p, d := range deleted {
		out = append(out, TaskFile{p, d})
	}
	slices.SortFunc(out, func(a, b TaskFile) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}
