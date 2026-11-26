package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/protocol/rpc"
)

// protocols are the transports a runner's git may use (GIT_ALLOW_PROTOCOL): never ext::, which runs a command.
var protocols = []string{"https", "ssh", "git", "file"}

// repoDir is where the runner keeps remote's clone: one per remote, which every worktree of it shares.
func repoDir(home, remote string) string {
	sum := sha256.Sum256([]byte(remote))
	return filepath.Join(home, "runner-repos", hex.EncodeToString(sum[:8]))
}

// HooksDir is the hooks folder of every worktree the runner makes (docs/several-machines.md).
func HooksDir(home string) string { return filepath.Join(home, "runner-hooks") }

// hookNames are the hooks the runner's folder holds: core.hooksPath takes the place of the project's own, so each runs the project's
// first. A hook git runs on every ref or index write is left out, as running it would cost each of those a shell.
var hookNames = []string{"applypatch-msg", "pre-applypatch", "post-applypatch", "pre-commit", "pre-merge-commit", "prepare-commit-msg",
	"commit-msg", "post-commit", "pre-rebase", "post-checkout", "post-merge", "pre-push", "post-rewrite", "pre-auto-gc",
	"push-to-checkout", "sendemail-validate"}

// hookScript runs the project's own hook of its name (its hooks folder read without the worktree's own config, which points here),
// and after a commit tells the runner, which pushes the branch. Telling never fails the commit.
const hookScript = `#!/bin/sh
name=$(basename "$0")
d=$(git config --local --get core.hooksPath || git config --global --get core.hooksPath || git config --system --get core.hooksPath)
case $d in
'') d="$(git rev-parse --git-common-dir)/hooks" ;;
"~/"*) d="$HOME/${d#"~/"}" ;;
/*) ;;
*) d="$(git rev-parse --show-toplevel)/$d" ;;
esac
rc=0
if [ -x "$d/$name" ] && ! [ "$d/$name" -ef "$0" ]; then
	"$d/$name" "$@"
	rc=$?
fi
if [ "$name" = post-commit ]; then
	%s hook git-commit </dev/null >/dev/null 2>&1
fi
exit $rc
`

// writeHooks writes the hooks folder, calling hl at hl; each hook is executable before it is in place.
func writeHooks(home, hl string) error {
	dir := HooksDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	script := []byte(fmt.Sprintf(hookScript, shellQuote(hl)))
	for _, name := range hookNames {
		tmp, err := os.CreateTemp(dir, "."+name+"-")
		if err != nil {
			return err
		}
		_, werr := tmp.Write(script)
		cerr := tmp.Close()
		if err := errors.Join(werr, cerr, os.Chmod(tmp.Name(), 0o755)); err != nil {
			os.Remove(tmp.Name())
			return err
		}
		if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
			os.Remove(tmp.Name())
			return err
		}
	}
	return nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// syncProject clones remote once, into a folder beside its place renamed into it when done, and fetches it each time after.
func (r *Runner) syncProject(remote string) error {
	if err := plain(remote); err != nil {
		return err
	}
	repo := repoDir(r.cfg.Home, remote)
	defer r.repoLock(repo)()
	return r.cloneOrFetch(remote, repo)
}

// cloneOrFetch is syncProject's work, with repo's lock held.
func (r *Runner) cloneOrFetch(remote, repo string) error {
	if _, err := os.Stat(repo); err == nil {
		return r.git("-C", repo, "fetch", "-q", "--prune", "origin")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(repo), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(repo), "."+filepath.Base(repo)+".clone-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	into := filepath.Join(tmp, "repo")
	if err := r.git("clone", "-q", "--no-checkout", "--", remote, into); err != nil {
		return err
	}
	return os.Rename(into, repo)
}

// repoLock holds repo against another use of it until the returned func.
func (r *Runner) repoLock(repo string) func() {
	r.mu.Lock()
	l := r.repos[repo]
	if l == nil {
		l = &sync.Mutex{}
		r.repos[repo] = l
	}
	r.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// makeWorktree makes a worktree of remote at path on branch, from base when the remote has no such branch (no base named is the
// remote's default branch), with the runner's hooks: each commit there is pushed. The clone is held for all of it, as two makes at
// once would race on its config and index.
func (r *Runner) makeWorktree(remote, path, branch, base string) (any, error) {
	if remote == "" {
		return nil, rpc.BadRequestError("remote is a string", "remote", rpc.InvalidParams)
	}
	named := []string{remote, path, branch}
	if base != "" {
		named = append(named, base)
	}
	if err := plain(named...); err != nil {
		return nil, err
	}
	repo := repoDir(r.cfg.Home, remote)
	defer r.repoLock(repo)()
	if err := r.cloneOrFetch(remote, repo); err != nil {
		return nil, err
	}
	if base == "" {
		base = gitrun.DefaultBranch(repo)
	}
	from := "origin/" + branch
	if r.git("-C", repo, "rev-parse", "--verify", "--quiet", "--end-of-options", from) != nil {
		from = "origin/" + base
	}
	if err := r.git("-C", repo, "config", "extensions.worktreeConfig", "true"); err != nil {
		return nil, err
	}
	if err := r.git("-C", repo, "worktree", "add", "-B", branch, "--", path, from); err != nil {
		return nil, err
	}
	if err := r.git("-C", path, "config", "--worktree", "core.hooksPath", HooksDir(r.cfg.Home)); err != nil {
		r.git("-C", repo, "worktree", "remove", "--force", "--", path) // a worktree whose commits would not be pushed is not left
		return nil, err
	}
	return map[string]any{"accepted": true, "path": path}, nil
}

// committed is runner.committed on the local socket, from the post-commit hook: the calling agent's worktree is pushed. Only the
// folder the master opened that agent's session in is: no other folder, and no other agent's.
func (r *Runner) committed(c *rpc.Conn, p map[string]any) (any, error) {
	if c.Caller.Kind != "agent" {
		return nil, rpc.Refused("runner.committed is an agent's own hook", "agent", "")
	}
	folder, _ := p["folder"].(string)
	r.mu.Lock()
	s, ok := r.sessions[c.Caller.Key]
	r.mu.Unlock()
	if !ok || s.Folder == "" || !sameFolder(folder, s.Folder) {
		return nil, rpc.Refused(folder+" is not the folder of "+c.Caller.Key+"'s session here", "folder", "")
	}
	r.push(s.Folder)
	return map[string]any{"accepted": true}, nil
}

func sameFolder(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, erra := filepath.EvalSymlinks(a)
	rb, errb := filepath.EvalSymlinks(b)
	return erra == nil && errb == nil && ra == rb
}

// pushWait is the longest wait between two tries of a push the remote does not take now; pushGiveUp is how long a push is tried.
const (
	pushWait   = time.Minute
	pushGiveUp = 30 * time.Minute
)

// pusher is one worktree's pushes: one runs at a time, and a commit made while it runs makes it push again once it is done.
type pusher struct{ again bool }

// push pushes folder's branch to origin, at once or after the push running there: the newest commit, again after a wait that grows to
// pushWait while the remote fails, for up to pushGiveUp. The agent's own rewrite (an amend, a rebase) goes over what it pushed before,
// never over a branch someone else moved (--force-with-lease against the last one fetched or pushed); a rejection is not tried again.
// A commit on no branch (a rebase under way) is not pushed: the next commit on the branch is.
func (r *Runner) push(folder string) {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	if p := r.pushers[folder]; p != nil {
		p.again = true
		r.mu.Unlock()
		return
	}
	r.pushers[folder] = &pusher{}
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		defer func() {
			r.mu.Lock()
			delete(r.pushers, folder)
			r.mu.Unlock()
		}()
		wait, end := time.Second, time.Now().Add(pushGiveUp)
		for {
			if r.git("-C", folder, "symbolic-ref", "-q", "HEAD") != nil {
				return
			}
			out, err := r.gitOut("-C", folder, "push", "--porcelain", "--force-with-lease", "-u", "origin", "HEAD")
			switch {
			case err == nil:
				wait, end = time.Second, time.Now().Add(pushGiveUp)
			case rejected(out):
				r.cfg.Log("runner: the push of %s was rejected, so it is left: %s", folder, out)
			case time.Now().After(end):
				r.cfg.Log("runner: the push of %s failed for %s, so it is left until the next commit: %v: %s", folder, pushGiveUp, err, out)
				return
			default:
				r.cfg.Log("runner: the push of %s failed, again in %s: %v: %s", folder, wait, err, out)
				select {
				case <-r.stop:
					return
				case <-time.After(wait):
				}
				wait = min(2*wait, pushWait)
				continue
			}
			r.mu.Lock()
			p := r.pushers[folder]
			again := p.again
			p.again = false
			r.mu.Unlock()
			if !again {
				return
			}
		}
	}()
}

// rejected is whether git push --porcelain says the remote refused a ref ("!"): the branch moved there, or the remote forbids it.
func rejected(porcelain string) bool {
	for _, l := range strings.Split(porcelain, "\n") {
		if strings.HasPrefix(l, "!\t") {
			return true
		}
	}
	return false
}

func (r *Runner) git(args ...string) error {
	if out, err := r.gitOut(args...); err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// gitTimeout bounds one git command the runner runs.
const gitTimeout = 10 * time.Minute

// gitOut runs git with no one at it, in C locale, until gitTimeout or the runner stops; out is what it said, trimmed.
func (r *Runner) gitOut(args ...string) (out string, err error) { return r.gitEnv(nil, args...) }

// gitEnv is gitOut with env added to git's environment; an error says what git said.
func (r *Runner) gitEnv(env []string, args ...string) (out string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	go func() {
		select {
		case <-r.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(append(os.Environ(), append(gitrun.NoPrompts(protocols), "LC_ALL=C")...), env...)
	b, err := cmd.CombinedOutput()
	if err != nil && env != nil {
		err = fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), err
}
