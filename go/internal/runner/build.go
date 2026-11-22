package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/atomicfile"
)

// BuildNames are the files of a build a master hands its runners: the daemon and the hl beside it.
var BuildNames = []string{"handlined", "hl"}

// BuildFile is one file of a build: its name, size and sha256.
type BuildFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Sum  string `json:"sum"`
}

// BuildOffer is the master's build a heartbeat's answer names to a runner it wants on it: its version and files.
type BuildOffer struct {
	Version string      `json:"version"`
	Files   []BuildFile `json:"files"`
}

// Platform is this build's GOOS/GOARCH: a master's build runs only on a runner of the same.
func Platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// BuildDir is where a runner keeps the master's build it took: one, the newest, replaced in place so the hl its hooks call stays.
func BuildDir(home string) string { return filepath.Join(home, "build") }

// buildVersion is the file in BuildDir naming the version kept there, written once its files are.
const buildVersion = "version"

// KeptBuild is the handlined of the build this home keeps for master version v, "" when it keeps none of v.
func KeptBuild(home, v string) string {
	b, err := os.ReadFile(filepath.Join(BuildDir(home), buildVersion))
	if err != nil || v == "" || strings.TrimSpace(string(b)) != v {
		return ""
	}
	return filepath.Join(BuildDir(home), "handlined")
}

// Again is closed once this runner took the master's build: main stops it and starts again on AgainExe.
func (r *Runner) Again() <-chan struct{} { return r.again }

// AgainExe is the handlined to start again on once Again is closed.
func (r *Runner) AgainExe() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.againExe
}

// takeBuild fetches the build o names over the link, checks each file's size and sha256, keeps it in BuildDir and has main start again
// on it. One at a time; a build refused is not fetched again until this daemon starts again, and its reason goes up in each heartbeat.
func (r *Runner) takeBuild(o BuildOffer) {
	if o.Version == r.cfg.Sha || !r.fetching.CompareAndSwap(false, true) {
		return
	}
	r.mu.Lock()
	refused := r.refusedBuild[o.Version] != ""
	r.mu.Unlock()
	if refused {
		r.fetching.Store(false)
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.cfg.Log("runner: the master runs Handline %s: taking its build", o.Version)
		if err := fetchBuild(r.cfg.Home, o, r.readPiece); err != nil {
			var bad *badBuild
			if !errors.As(err, &bad) { // the link ended, or the disk: the next heartbeat's answer tries again
				r.cfg.Log("runner: the master's build %s did not arrive: %v", o.Version, err)
				r.fetching.Store(false)
				return
			}
			r.cfg.Log("runner: the master's build %s was refused: %v", o.Version, err)
			r.mu.Lock()
			r.refusedBuild[o.Version] = err.Error()
			r.mu.Unlock()
			r.fetching.Store(false)
			r.beatSoon()
			return
		}
		r.mu.Lock()
		r.againExe = KeptBuild(r.cfg.Home, o.Version)
		r.mu.Unlock()
		r.cfg.Log("runner: took the master's build %s: starting again on it", o.Version)
		r.againOnce.Do(func() { close(r.again) })
	}()
}

// badBuild is a build that came whole but is not what the master named: refused until this daemon starts again.
type badBuild struct{ why string }

func (b *badBuild) Error() string { return b.why }

// fetchBuild reads o's files with read into a folder beside home's BuildDir, checks them, and renames each into BuildDir, its version
// file last.
func fetchBuild(home string, o BuildOffer, read func(name string, off int64) ([]byte, error)) error {
	names := map[string]bool{}
	for _, f := range o.Files {
		if !slices.Contains(BuildNames, f.Name) || names[f.Name] {
			return &badBuild{fmt.Sprintf("the master's build names the file %q", f.Name)}
		}
		names[f.Name] = true
	}
	if len(names) != len(BuildNames) {
		return &badBuild{"the master's build lacks a file of " + strings.Join(BuildNames, " and ")}
	}
	dir := BuildDir(home)
	tmp := dir + ".part"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	for _, f := range o.Files {
		if err := fetchFile(filepath.Join(tmp, f.Name), f, read); err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, buildVersion)); err != nil && !os.IsNotExist(err) { // it names no files half replaced
		return err
	}
	for _, n := range BuildNames { // a running handlined keeps the file it started from; a session's hook calls the new hl
		if err := os.Rename(filepath.Join(tmp, n), filepath.Join(dir, n)); err != nil {
			return err
		}
	}
	return atomicfile.Write(filepath.Join(dir, buildVersion), o.Version+"\n")
}

// fetchFile reads f in pieces with read into path and checks its size and sha256.
func fetchFile(path string, f BuildFile, read func(name string, off int64) ([]byte, error)) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	h := sha256.New()
	var off int64
	for off < f.Size {
		data, err := read(f.Name, off)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return fmt.Errorf("the master sent %d of its %d bytes", off, f.Size)
		}
		if off+int64(len(data)) > f.Size {
			return &badBuild{fmt.Sprintf("the master sent more than its %d bytes", f.Size)}
		}
		h.Write(data)
		if _, err := out.Write(data); err != nil {
			return err
		}
		off += int64(len(data))
	}
	if sum := hex.EncodeToString(h.Sum(nil)); sum != f.Sum {
		return &badBuild{fmt.Sprintf("its sha256 is %s, not the %s the master named", sum, f.Sum)}
	}
	return out.Close()
}

// readPiece is one node.build.read over the link.
func (r *Runner) readPiece(name string, off int64) ([]byte, error) {
	l := r.linked()
	if l == nil {
		return nil, errors.New("the link to the master ended")
	}
	got, err := l.Call("node.build.read", map[string]any{"name": name, "offset": off}, hlclient.Quick)
	if err != nil {
		return nil, err
	}
	var piece struct {
		Data []byte `json:"data"`
	}
	if b, err := json.Marshal(got); err != nil || json.Unmarshal(b, &piece) != nil {
		return nil, errors.New("the master's answer is not a piece of the file")
	}
	return piece.Data, nil
}
