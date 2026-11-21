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

// IntegrationBranch is the branch work is branched from and merged into.
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

// DefaultBranch is the branch the repository at repo merges into: origin's HEAD, else main or master when it has one, else the
// branch its main worktree has checked out, else IntegrationBranch. repo may be any of its worktrees.
func DefaultBranch(repo string) string {
	if r := Run(repo, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"); r.RC == 0 && strings.TrimSpace(r.Stdout) != "" {
		return strings.TrimPrefix(strings.TrimSpace(r.Stdout), "origin/")
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

// PointOriginHeadAtMain makes origin/HEAD name origin/main. True when it was changed; false when it already did, or when
// there is no origin/main to point at.
func PointOriginHeadAtMain(cwd string) bool {
	r := Run(cwd, "symbolic-ref", "-q", "refs/remotes/origin/HEAD")
	if r.RC == 0 && strings.TrimSpace(r.Stdout) == "refs/remotes/origin/"+IntegrationBranch {
		return false
	}
	return Run(cwd, "remote", "set-head", "origin", IntegrationBranch).RC == 0
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
	if def := DefaultBranch(repo); Run(repo, "rev-parse", "--verify", "--quiet", "origin/"+def+"^{commit}").RC == 0 {
		revs = append(revs, "origin/"+def)
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
