// Package gitrun runs git and hands back what it said, the same way for every caller (handline/infra/gitrun.py).
package gitrun

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
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
