package adminwrites

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// folder.init and folder.clone: the two ways the first-run folder pick turns a folder that is not a repository into one. Both wait for git
// (Waits), and the caller hanging up stops it.

// secretExcludes keep secrets out of the first commit and of every later add; they go to .git/info/exclude, never to a file of the folder.
var secretExcludes = []string{".env", ".env.*", "!.env.example", "*.pem", "*.p12", "id_rsa*", "id_ed25519*", ".DS_Store"}

const (
	initLimit  = 4 * time.Minute // git add and commit of a big folder
	cloneLimit = 7 * time.Minute // under hl's deadline for the call (hlclient.Timeout)
)

// cloneProtocols are the transports a clone may use (GIT_ALLOW_PROTOCOL): never ext::, which runs a command, nor a local path. A test adds
// file to clone from a folder.
var cloneProtocols = []string{"https", "ssh", "git"}

func (e *Env) folderInit(c *rpc.Conn, p map[string]any) (any, error) {
	path, _ := p["path"].(string)
	if fi, err := os.Stat(path); !filepath.IsAbs(path) || err != nil || !fi.IsDir() {
		nf := rpc.NotFoundError(path + " is not a folder")
		nf.Data = map[string]any{"what": "path", "path": path}
		return nil, nf
	}
	path = filepath.Clean(path)
	if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
		return nil, refused(path+" is already a git repository", "state")
	}
	if home := paths.UserHome(); home != "" && isOrAbove(path, home) {
		return nil, refused(path+" holds the home folder: choose a project folder inside it", "state")
	}
	if r := gitrun.Run(path, "rev-parse", "--show-toplevel"); r.RC == 0 {
		return nil, refused(fmt.Sprintf("%s is inside the git repository %s", path, strings.TrimSpace(r.Stdout)), "state")
	}
	ctx, cancel := callContext(c, initLimit)
	defer cancel()
	branch, why := firstCommit(ctx, path)
	if why != "" {
		if err := os.RemoveAll(filepath.Join(path, ".git")); err != nil {
			why += "; its .git could not be removed: " + err.Error()
		}
		return nil, refused(path+": "+why, "state")
	}
	return wrote(c, fmt.Sprintf("%s is a git repository on %s with a first commit", path, branch), map[string]any{"path": path})
}

// firstCommit runs git init and commits what the folder holds, secrets excluded; it answers the branch, or why it failed.
func firstCommit(ctx context.Context, path string) (branch, why string) {
	args := []string{"init", "-q"}
	if gitrun.Run(path, "config", "--get", "init.defaultBranch").RC != 0 { // not a repository yet: system and global config
		args = append(args, "--initial-branch="+gitrun.IntegrationBranch)
	}
	if err := runGit(ctx, path, nil, args...); err != nil {
		return "", "git init failed: " + err.Error()
	}
	exclude := filepath.Join(path, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return "", err.Error()
	}
	f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err.Error()
	}
	_, err = f.WriteString("\n# secrets Handline keeps out of commits\n" + strings.Join(secretExcludes, "\n") + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err.Error()
	}
	if err := runGit(ctx, path, nil, "add", "-A"); err != nil {
		return "", "git add failed: " + err.Error()
	}
	commit := append(identity(path), "-c", "commit.gpgsign=false", "commit", "-q", "--no-verify", "--allow-empty", "-m", "First commit")
	if err := runGit(ctx, path, nil, commit...); err != nil {
		return "", "git commit failed: " + err.Error()
	}
	return strings.TrimSpace(gitrun.Run(path, "branch", "--show-current").Stdout), ""
}

// identity is the Mac user as the committer when git has no user.name or user.email: git would refuse the commit otherwise.
func identity(path string) []string {
	login, name := "handline", ""
	if u, err := user.Current(); err == nil { // no user record: the fallback names stand
		login, name = u.Username, u.Name
	}
	if name == "" {
		name = login
	}
	var out []string
	if gitrun.Run(path, "config", "--get", "user.name").RC != 0 {
		out = append(out, "-c", "user.name="+name)
	}
	if gitrun.Run(path, "config", "--get", "user.email").RC != 0 {
		host, err := os.Hostname()
		if err != nil || host == "" {
			host = "localhost"
		}
		out = append(out, "-c", "user.email="+login+"@"+host)
	}
	return out
}

func (e *Env) folderClone(c *rpc.Conn, p map[string]any) (any, error) {
	src, _ := p["url"].(string)
	src = strings.TrimSpace(src)
	if src == "" || strings.HasPrefix(src, "-") || textsafe.HasUnsafe(src) || !slices.Contains(cloneProtocols, transport(src)) {
		return nil, badParam("url is a repository's https, ssh or git address", "url")
	}
	path, given, err := text(p, "path", false)
	if err != nil {
		return nil, err
	}
	if !given {
		name, home := repoName(src), paths.UserHome()
		if name == "" {
			return nil, badParam("url ends in no repository name to call its folder: give path", "url")
		}
		if home == "" {
			return nil, badParam("this Mac user has no home folder to clone under: give path", "path")
		}
		path = filepath.Join(home, "Handline", name)
	}
	if !filepath.IsAbs(path) {
		return nil, badParam("path is an absolute path", "path")
	}
	path = filepath.Clean(path)
	if ents, err := os.ReadDir(path); err == nil && len(ents) > 0 {
		return nil, refused(path+" already exists and is not empty: choose another folder", "state")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, refused(path+" exists and is not an empty folder: choose another folder", "state")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, refused(err.Error(), "state")
	}
	// git clones into a folder of its own beside path, renamed to path when done: a failure removes only that, and a rename onto a
	// folder another call filled meanwhile fails instead of mixing the two.
	tmp, err := os.MkdirTemp(filepath.Dir(path), "."+filepath.Base(path)+".clone-")
	if err != nil {
		return nil, refused(err.Error(), "state")
	}
	defer os.RemoveAll(tmp)
	ctx, cancel := callContext(c, cloneLimit)
	defer cancel()
	into := filepath.Join(tmp, "repo")
	if err := runGit(ctx, "", noPrompts(), "clone", "-q", "--", src, into); err != nil {
		return nil, refused("git clone failed: "+err.Error(), "state")
	}
	os.Remove(path) // the empty folder given, if any: os.Rename takes no folder's place, and Remove takes no folder that is not empty
	if err := os.Rename(into, path); err != nil {
		return nil, refused(path+" was filled while the clone ran: choose another folder", "state")
	}
	return wrote(c, shown(src)+" is cloned into "+path, map[string]any{"path": path})
}

// noPrompts is the environment of a git the daemon runs with no one at it: neither git nor ssh asks for anything, so what needs a
// password or a new host key fails at once; and git uses no transport but cloneProtocols, the submodules' and redirects' too.
func noPrompts() []string {
	env := []string{"GIT_ALLOW_PROTOCOL=" + strings.Join(cloneProtocols, ":"), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_ASKPASS=", "SSH_ASKPASS="}
	if os.Getenv("GIT_SSH_COMMAND") == "" && gitrun.Run("", "config", "--get", "core.sshCommand").RC != 0 {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}

// callContext ends at limit or when the caller hangs up.
func callContext(c *rpc.Conn, limit time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	go func() {
		select {
		case <-c.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// runGit runs git in its own process group, so a stop also stops the helpers it started (git-remote-https, index-pack); the error is
// git's last line.
func runGit(ctx context.Context, dir string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return errors.New("stopped before it ended")
	}
	if line := lastLine(stderr.String()); line != "" {
		return errors.New(line)
	}
	return err
}

// transport is the protocol git would clone src over: the scheme of scheme://, the helper of helper::, ssh for user@host:path, file for a
// local path.
func transport(src string) string {
	if i := strings.Index(src, "://"); i > 0 {
		return strings.ToLower(src[:i])
	}
	if i := strings.Index(src, "::"); i > 0 {
		return strings.ToLower(src[:i])
	}
	if colon := strings.IndexByte(src, ':'); colon > 0 && !strings.Contains(src[:colon], "/") {
		return "ssh"
	}
	return "file"
}

// shown is the url with any user and password taken out, to be said and kept.
func shown(src string) string {
	if u, err := url.Parse(src); err == nil && u.User != nil {
		u.User = nil
		return u.String()
	}
	return src
}

// repoName is the url's last part less .git: the folder name git clone would choose.
func repoName(src string) string {
	s := strings.TrimRight(src, "/")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(s, ".git")
	if s == "" || s == "." || s == ".." {
		return ""
	}
	return s
}

// isOrAbove: path is dir or one of its parents.
func isOrAbove(path, dir string) bool {
	rel, err := filepath.Rel(path, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
