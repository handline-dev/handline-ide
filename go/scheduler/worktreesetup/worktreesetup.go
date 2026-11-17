// Package worktreesetup is what a project asks of every worktree the daemon makes for it: files copied from its main checkout (an
// .env git does not track) and a setup script (pnpm install), run once when the worktree is made. The founder sets both, per repo, in
// the protected store: the script runs outside any sandbox, so no agent may write it.
package worktreesetup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/kit/textsafe"
)

// NS is the protected namespace of the setups: one entry per repo, named by Key.
const NS = "repo_setup"

const (
	// ScriptMax is the longest script kept, in characters.
	ScriptMax = 4000
	// CopyMax is how many copy entries a repo may have, and PathMax how long each may be.
	CopyMax = 50
	PathMax = 300
	// OutputMax is how much of the script's output a result keeps: its end, where the error is.
	OutputMax = 4000
)

// ScriptLimit is how long the script may run before it is stopped.
const ScriptLimit = 5 * time.Minute

// Setup is one repo's: Copy are paths relative to the repo's folder, a glob each (filepath.Match), copied into the new worktree; Script
// runs in it after the copy.
type Setup struct {
	Script string   `json:"script"`
	Copy   []string `json:"copy"`
}

// Empty is whether the setup asks for nothing.
func (s Setup) Empty() bool { return strings.TrimSpace(s.Script) == "" && len(s.Copy) == 0 }

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]+`)

// Key is the store's name for repo: its folder's name, then a hash of its resolved path, so two repos never share one.
func Key(repo string) string {
	if real, err := filepath.EvalSymlinks(repo); err == nil {
		repo = real
	}
	repo = filepath.Clean(repo)
	sum := sha256.Sum256([]byte(repo))
	return strings.Trim(nonAlnum.ReplaceAllString(filepath.Base(repo), "-"), "-") + "-" + hex.EncodeToString(sum[:6])
}

// FieldError is a setup Check refuses, naming its field: script or copy.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

// Get is repo's setup; none, or one that cannot be read, asks for nothing.
func Get(st *protected.Store, repo string) Setup {
	var s Setup
	if st == nil {
		return s
	}
	if v, ok := st.Get(NS, Key(repo)); ok {
		_ = json.Unmarshal([]byte(v), &s)
	}
	if s.Copy == nil {
		s.Copy = []string{}
	}
	return s
}

// Put records repo's setup, after Check; an empty one removes it.
func Put(st *protected.Store, repo string, s Setup) error {
	if err := Check(s); err != nil {
		return err
	}
	if s.Empty() {
		return st.Delete(NS, Key(repo))
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return st.Put(NS, Key(repo), string(b))
}

// Check is why s cannot be kept: a script too long or holding a NUL, a copy entry that is not a relative path inside the repo, reaches
// into .git, is listed twice or is not a valid glob.
func Check(s Setup) error {
	if utf8.RuneCountInString(s.Script) > ScriptMax || strings.ContainsRune(s.Script, 0) {
		return &FieldError{"script", fmt.Sprintf("script is text of at most %d characters", ScriptMax)}
	}
	if len(s.Copy) > CopyMax {
		return &FieldError{"copy", fmt.Sprintf("copy lists at most %d paths", CopyMax)}
	}
	seen := map[string]bool{}
	for _, p := range s.Copy {
		bad := p == "" || len(p) > PathMax || textsafe.HasUnsafe(p) || filepath.IsAbs(p) || seen[p]
		if !bad {
			c := filepath.Clean(p)
			_, globErr := filepath.Match(p, "")
			bad = c == "." || c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator)) || inGit(c) || globErr != nil
		}
		if bad {
			return &FieldError{"copy", fmt.Sprintf("copy path %q is not a path inside the repo (relative, at most %d characters, not in .git, listed once)",
				p, PathMax)}
		}
		seen[p] = true
	}
	return nil
}

// inGit is whether rel, a clean path relative to the repo, is .git or in it.
func inGit(rel string) bool { return strings.SplitN(rel, string(filepath.Separator), 2)[0] == ".git" }

// Result is one run of a setup in a new worktree. Missing are copy entries the main checkout had nothing for: told, not a failure.
type Result struct {
	At       string   `json:"at"`
	Worktree string   `json:"worktree"`
	OK       bool     `json:"ok"`
	Copied   []string `json:"copied"`
	Missing  []string `json:"missing"`
	Error    string   `json:"error"`
	Output   string   `json:"output"`
}

// Said is the result in one line, for the spawn's report.
func (r Result) Said() string {
	var parts []string
	if len(r.Copied) > 0 {
		parts = append(parts, fmt.Sprintf("copied %s", strings.Join(r.Copied, ", ")))
	}
	if len(r.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("not in the main checkout: %s", strings.Join(r.Missing, ", ")))
	}
	if !r.OK {
		return "setup FAILED: " + strings.Join(append(parts, r.Error), "; ")
	}
	if len(parts) == 0 {
		return "setup done"
	}
	return "setup done: " + strings.Join(parts, "; ")
}

// Run copies s's files from repo into wt, then runs its script in wt; a failed copy runs no script. The script runs in the user's login
// shell ($SHELL -lc, so pnpm and node are on its PATH), with HANDLINE_REPO and HANDLINE_WORKTREE set, stdin empty, for at most limit.
func Run(repo, wt string, s Setup, now time.Time, limit time.Duration) Result {
	r := Result{At: now.UTC().Format(time.RFC3339), Worktree: wt, OK: true, Copied: []string{}, Missing: []string{}}
	for _, pat := range s.Copy {
		matches, _ := filepath.Glob(filepath.Join(repo, pat))
		if len(matches) == 0 {
			r.Missing = append(r.Missing, pat)
			continue
		}
		for _, src := range matches {
			rel, err := filepath.Rel(repo, src)
			if err == nil && inGit(rel) { // a glob such as .* matches .git
				continue
			}
			wrote := false
			if err == nil {
				wrote, err = copyNew(src, wt, rel)
			}
			if err != nil {
				r.OK, r.Error = false, fmt.Sprintf("copying %s: %v", pat, err)
				return r
			}
			if wrote {
				r.Copied = append(r.Copied, rel)
			}
		}
	}
	if strings.TrimSpace(s.Script) == "" {
		return r
	}
	out, err := runScript(repo, wt, s.Script, limit)
	r.Output = tail(out)
	if err != nil {
		r.OK, r.Error = false, "script "+err.Error()
	}
	return r
}

func shell() string {
	if sh := os.Getenv("SHELL"); filepath.IsAbs(sh) {
		return sh
	}
	return "/bin/sh"
}

func runScript(repo, wt, script string, limit time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell(), "-l", "-c", script)
	cmd.Dir = wt
	cmd.Env = append(os.Environ(), "HANDLINE_REPO="+repo, "HANDLINE_WORKTREE="+wt)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	// Its own process group, so what it started (pnpm's children, a build daemon left behind) is stopped with it: on a timeout, and once
	// it has exited.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if ctx.Err() != nil {
		return out.String(), fmt.Errorf("stopped after %s", limit)
	}
	if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState.Success() {
		err = nil // it exited 0; only something it left running held the output open
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), fmt.Errorf("exited %d", ee.ExitCode())
	}
	if err != nil {
		return out.String(), fmt.Errorf("did not run: %v", err)
	}
	return out.String(), nil
}

func tail(s string) string {
	s = strings.ToValidUTF8(s, "")
	if len(s) <= OutputMax {
		return s
	}
	s = s[len(s)-OutputMax:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}

// copyNew copies src to wt/rel, a folder with what it holds, and is whether it wrote anything. What the worktree already has there is its
// own and kept: a folder is walked into for what it lacks, anything else is left. Nothing is written through a link in the worktree, which
// its branch may hold pointing anywhere.
func copyNew(src, wt, rel string) (bool, error) {
	wrote := false
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		under, _ := filepath.Rel(src, p)
		to := filepath.Join(wt, rel, under)
		if err := noLinkBetween(wt, filepath.Dir(to)); err != nil {
			return err
		}
		if fi, err := os.Lstat(to); err == nil {
			if d.IsDir() && !fi.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		wrote = true
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.Mkdir(to, info.Mode().Perm())
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(target, to)
		case d.Type().IsRegular():
			return copyFile(p, to, info.Mode().Perm())
		}
		return nil // a socket or a device is not copied
	})
	return wrote, err
}

// noLinkBetween refuses dir, inside wt, when a folder on the way from wt to it is a link.
func noLinkBetween(wt, dir string) error {
	rel, err := filepath.Rel(wt, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is outside the worktree", dir)
	}
	at := wt
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		at = filepath.Join(at, part)
		fi, err := os.Lstat(at)
		if err != nil {
			return nil // not there yet: made as a folder
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s in the worktree is a link: not copied through it", at)
		}
	}
	return nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// lastKey is the meta name of repo's last result.
func lastKey(repo string) string { return "repo_setup_last:" + Key(repo) }

// Record keeps r as repo's last result.
func Record(q db.Q, repo string, r Result) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return db.SetMeta(q, lastKey(repo), string(b))
}

// Last is repo's last result, nil when no setup ran there.
func Last(q db.Q, repo string) (*Result, error) {
	v, ok, err := db.Meta(q, lastKey(repo))
	if err != nil || !ok || v == "" {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal([]byte(v), &r); err != nil {
		return nil, nil // a record that cannot be read is no record
	}
	return &r, nil
}
