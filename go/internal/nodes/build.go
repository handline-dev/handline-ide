package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/protocol/rpc"
)

// BuildAutoKey is whether a lent machine on another Handline build than the master, of the master's OS and architecture, takes the
// master's own build and starts again on it (Auto, true, the default); Manual (false) waits for its row's Update (device.update).
const BuildAutoKey = "node_build_auto"

// BuildChunk bounds the data one node.build.read answer carries, as CopyChunk does a copy's.
const BuildChunk = CopyChunk

// Build is the handlined and hl this master runs, as it serves them to its runners (node.build.read): each opened at the start, so a
// file rebuilt in its place by a rename later is not what is served under this version; summed when first offered.
type Build struct {
	Platform string // GOOS/GOARCH
	files    map[string]*os.File
	mu       sync.Mutex
	sums     []runner.BuildFile
}

// OpenBuild opens handlined at exe and the hl beside it.
func OpenBuild(exe string) (*Build, error) {
	b := &Build{Platform: runner.Platform(), files: map[string]*os.File{}}
	for _, name := range runner.BuildNames {
		path := exe
		if name != "handlined" {
			path = filepath.Join(filepath.Dir(exe), name)
		}
		f, err := os.Open(path)
		if err != nil {
			b.Close()
			return nil, err
		}
		b.files[name] = f
	}
	return b, nil
}

// Files are its files' names, sizes and sha256 sums, summed once.
func (b *Build) Files() ([]runner.BuildFile, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sums != nil {
		return b.sums, nil
	}
	var sums []runner.BuildFile
	for _, name := range runner.BuildNames {
		h := sha256.New()
		n, err := io.Copy(h, io.NewSectionReader(b.files[name], 0, 1<<62))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		sums = append(sums, runner.BuildFile{Name: name, Size: n, Sum: hex.EncodeToString(h.Sum(nil))})
	}
	b.sums = sums
	return sums, nil
}

// Close closes its files.
func (b *Build) Close() {
	for _, f := range b.files {
		f.Close()
	}
}

// offer is the build a heartbeat's answer names to node, nil when it starts on none now: the master serves none, the runner is on the
// master's build already or on another platform, or its Update was not asked and Auto is off or it is not the owner's.
func (r *Registry) offer(node string, beat runner.Beat) *runner.BuildOffer {
	if r.Build == nil || r.Version == "" || beat.Version == r.Version || beat.Platform != r.Build.Platform {
		return nil
	}
	r.mu.Lock()
	asked := r.updates[node]
	r.mu.Unlock()
	if !asked { // Auto: only for the owner's machines; another person's takes it at its own person's Update
		if owner, err := r.ownerNode(node); err != nil || !owner || r.BuildAuto == nil || !r.BuildAuto() {
			return nil
		}
	}
	files, err := r.Build.Files()
	if err != nil {
		if r.Log != nil {
			r.Log("nodes: the master's build is not offered: %v", err)
		}
		return nil
	}
	return &runner.BuildOffer{Version: r.Version, Files: files}
}

// appUpdate is the app version a heartbeat's answer has node's Mac app install, nil for none: the newer one its app has ready, once its
// row's Update asked for it, or by itself (Auto) on the owner's Mac with node_build_auto on, when no window is open there.
func (r *Registry) appUpdate(node string, b runner.Beat) *runner.AppUpdate {
	r.mu.Lock()
	asked := r.appAsks[node]
	if b.App == nil || b.App.Ready == "" {
		delete(r.appAsks, node) // installed, or none ready any more
	}
	r.mu.Unlock()
	if b.App == nil || b.App.Ready == "" {
		return nil
	}
	if asked == b.App.Ready {
		return &runner.AppUpdate{Version: b.App.Ready}
	}
	if owner, err := r.ownerNode(node); err == nil && owner && r.BuildAuto != nil && r.BuildAuto() {
		return &runner.AppUpdate{Version: b.App.Ready, Auto: true}
	}
	return nil
}

// AskUpdate has node's next heartbeat answer name the master's build, as Auto would (the Manual row's Update), and the app version its
// Mac app has ready; refused when node can take neither.
func (r *Registry) AskUpdate(node string) error {
	b, _, ok := r.Heard(node)
	if !ok || !r.Online(node) {
		return rpc.Refused("it is offline", "build", "")
	}
	build := r.buildRefused(b)
	ready := ""
	if b.App != nil {
		ready = b.App.Ready
	}
	if build != nil && ready == "" {
		return build
	}
	r.mu.Lock()
	if build == nil {
		r.updates[node] = true
	}
	if ready != "" {
		r.appAsks[node] = ready
	}
	r.mu.Unlock()
	return nil
}

// buildRefused is why node, last heard as b, cannot take the master's build, nil when it can.
func (r *Registry) buildRefused(b runner.Beat) error {
	switch {
	case r.Build == nil:
		return rpc.Refused("this master serves no build of its own", "build", "")
	case b.Version == r.Version:
		return rpc.Refused("it runs the master's build already", "build", "")
	case b.Platform != r.Build.Platform:
		return rpc.Refused(fmt.Sprintf("it is %s, not the master's %s: install Handline there", orUnknown(b.Platform), r.Build.Platform), "build", "")
	}
	return nil
}

func orUnknown(s string) string {
	if s == "" {
		return "an older Handline"
	}
	return s
}

// buildRead is node.build.read: up to BuildChunk bytes of one file of the master's build, from offset.
func (r *Registry) buildRead(c *rpc.Conn, p map[string]any) (any, error) {
	if r.NodeOf(c.Root()) == "" {
		return nil, rpc.Refused("node.build.read is a runner's own call, on its link", "node", "")
	}
	if r.Build == nil {
		return nil, rpc.Refused("this master serves no build of its own", "build", "")
	}
	name, _ := p["name"].(string)
	f := r.Build.files[name]
	if f == nil {
		return nil, rpc.BadRequestError("name is handlined or hl", "name", rpc.InvalidParams)
	}
	off, ok := rpc.AsInt(p["offset"])
	if !ok || off < 0 {
		return nil, rpc.BadRequestError("offset is a whole number of bytes", "offset", rpc.InvalidParams)
	}
	buf := make([]byte, BuildChunk)
	n, err := f.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return map[string]any{"data": buf[:n]}, nil
}
