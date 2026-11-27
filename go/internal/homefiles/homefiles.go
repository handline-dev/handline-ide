// Package homefiles is files of a Handline home sent between the master and a runner (docs/several-machines.md): each named by
// its path under the home, so it lands at the same place under the other machine's home, and never outside it.
package homefiles

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/kit/atomicfile"
	"github.com/handline/handline/go/protocol/rpc"
)

// File is one file: its path under the home, what it holds and its mode bits. A piece of a file too big for one message has Size, the
// whole file's, and Offset, where Data goes in it.
type File struct {
	Path   string
	Data   []byte
	Mode   fs.FileMode
	Offset int64
	Size   int64
}

// Rel is abs as a path under home; an error when it is not under it.
func Rel(home, abs string) (string, error) {
	rel, err := filepath.Rel(home, abs)
	if err != nil || !Inside(rel) {
		return "", fmt.Errorf("%s is not under %s", abs, home)
	}
	return rel, nil
}

// Inside is whether rel names a file under a home: relative, clean and with no part above it.
func Inside(rel string) bool {
	return rel != "" && rel != "." && !filepath.IsAbs(rel) && filepath.Clean(rel) == rel && rel != ".." &&
		!strings.HasPrefix(rel, "../")
}

// Collect is every file at the paths named under home, a folder by every file in it; a path that is not there is skipped, and so is a
// link: what it points at may be anywhere.
func Collect(home string, abs ...string) ([]File, error) {
	var out []File
	for _, p := range abs {
		if filepath.Clean(p) != p {
			return nil, fmt.Errorf("%s is not a clean path", p)
		}
		if _, err := Rel(home, p); err != nil {
			return nil, err
		}
		err := filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if !d.Type().IsRegular() || isPart(path) {
				return nil // a folder is walked; a link, a socket, a file still coming in pieces: nothing to send
			}
			st, err := d.Info()
			if err != nil {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := Rel(home, path)
			if err != nil {
				return err
			}
			out = append(out, File{Path: rel, Data: b, Mode: st.Mode().Perm()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Apply writes files under home and removes deleted (paths under home), refusing any path outside it before it touches anything. A
// link on the way, or in a file's place, is refused: it could lead out of the home.
func Apply(home string, files []File, deleted []string) error {
	for _, f := range files {
		if !Inside(f.Path) {
			return fmt.Errorf("%q is not a path under the home", f.Path)
		}
	}
	for _, d := range deleted {
		if !Inside(d) {
			return fmt.Errorf("%q is not a path under the home", d)
		}
	}
	real, err := filepath.EvalSymlinks(home)
	if err != nil {
		return err
	}
	for _, f := range files {
		path, err := within(home, real, f.Path, true)
		if err != nil {
			return err
		}
		if f.Size > 0 {
			whole, err := writePiece(home, real, path, f)
			if err != nil {
				return err
			}
			if !whole {
				continue
			}
		} else if err := atomicfile.Write(path, string(f.Data)); err != nil {
			return err
		}
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
	}
	for _, d := range deleted {
		path, err := within(home, real, d, false)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		os.Remove(filepath.Join(filepath.Dir(path), filepath.Base(partOf(d)))) // a send of it cut short
	}
	return nil
}

// writePiece writes f, a piece, into path's part file, made anew by the first piece, and renames it to path once it is whole.
func writePiece(home, real, path string, f File) (whole bool, err error) {
	part, err := within(home, real, partOf(f.Path), false)
	if err != nil {
		return false, err
	}
	flag := os.O_WRONLY
	if f.Offset == 0 {
		flag |= os.O_CREATE | os.O_TRUNC
	} else if st, err := os.Stat(part); err != nil || st.Size() != f.Offset {
		return false, fmt.Errorf("%s: a piece came out of its order", f.Path)
	}
	out, err := os.OpenFile(part, flag, 0o600)
	if err != nil {
		return false, fmt.Errorf("%s: a piece came without its start: %w", f.Path, err)
	}
	_, err = out.WriteAt(f.Data, f.Offset)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil || f.Offset+int64(len(f.Data)) < f.Size {
		return false, err
	}
	return true, os.Rename(part, path)
}

// partOf is where the pieces of the file at rel gather until it is whole: hidden beside it, under a name no tool writes.
func partOf(rel string) string {
	return filepath.Join(filepath.Dir(rel), "."+filepath.Base(rel)+".hl-part")
}

func isPart(path string) bool { return strings.HasSuffix(path, ".hl-part") }

// within is rel's path under home, its folder made when make, once no link on the way leads out of the home (real, home with its own
// links resolved) and none stands in the file's place.
func within(home, real, rel string, make bool) (string, error) {
	path := filepath.Join(home, rel)
	dir := filepath.Dir(path)
	if make {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if r, err := filepath.Rel(real, resolved); err != nil || (r != "." && !Inside(r)) {
		return "", fmt.Errorf("%s leads out of the home", rel)
	}
	if st, err := os.Lstat(path); err == nil && st.Mode()&fs.ModeSymlink != 0 {
		return "", fmt.Errorf("%s is a link", rel)
	}
	return path, nil
}

// Wire is files as a call carries them.
func Wire(files []File) []any {
	out := make([]any, 0, len(files))
	for _, f := range files {
		w := map[string]any{"path": f.Path, "data": base64.StdEncoding.EncodeToString(f.Data), "mode": int(f.Mode)}
		if f.Size > 0 {
			w["offset"], w["size"] = f.Offset, f.Size
		}
		out = append(out, w)
	}
	return out
}

// MessageBytes is the most a piece of a file carries; a message of Messages is at most about messageWire bytes on the wire, under
// rpc.MaxLineBytes, since a message past that ends the link it came on.
const (
	MessageBytes = 512 << 10
	messageWire  = 768 << 10
)

// wireCost is about the most an entry takes in a message: its base64, its path with each byte escaped, and the JSON around them.
func wireCost(data int, path string) int {
	return base64.StdEncoding.EncodedLen(data) + 6*len(path) + 64
}

// Messages is files and deleted as the params of calls of at most about MessageBytes each, in order: a file bigger than that goes as
// pieces, each in a message of its own, and the deleted paths in the last messages.
func Messages(files []File, deleted []string) []map[string]any {
	var out []map[string]any
	var batch []File
	var gone []any
	n := 0
	flush := func() {
		if len(batch) > 0 || len(gone) > 0 {
			out = append(out, map[string]any{"files": Wire(batch), "deleted": append([]any{}, gone...)})
		}
		batch, gone, n = nil, nil, 0
	}
	for _, f := range files {
		if wireCost(len(f.Data), f.Path) > messageWire {
			flush()
			for off := 0; off < len(f.Data); off += MessageBytes {
				p := f
				p.Data, p.Offset, p.Size = f.Data[off:min(off+MessageBytes, len(f.Data))], int64(off), int64(len(f.Data))
				batch = []File{p}
				flush()
			}
			continue
		}
		if c := wireCost(len(f.Data), f.Path); n+c > messageWire {
			flush()
		}
		batch = append(batch, f)
		n += wireCost(len(f.Data), f.Path)
	}
	for _, d := range deleted {
		if n+wireCost(0, d) > messageWire {
			flush()
		}
		gone = append(gone, d)
		n += wireCost(0, d)
	}
	flush()
	return out
}

// FromWire is the files a call carried.
func FromWire(v any) ([]File, error) {
	list, ok := v.([]any)
	if v != nil && !ok {
		return nil, errors.New("files is a list")
	}
	out := make([]File, 0, len(list))
	for _, it := range list {
		m, _ := it.(map[string]any)
		path, _ := m["path"].(string)
		data, _ := m["data"].(string)
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil || path == "" {
			return nil, errors.New("a file is a path and its base64 data")
		}
		mode, _ := rpc.AsInt(m["mode"])
		f := File{Path: path, Data: b, Mode: fs.FileMode(mode) & fs.ModePerm}
		if size, ok := rpc.AsInt(m["size"]); ok {
			off, _ := rpc.AsInt(m["offset"])
			if size <= 0 || off < 0 || off+int64(len(b)) > size {
				return nil, errors.New("a piece of a file reaches past its size")
			}
			f.Offset, f.Size = off, size
		}
		out = append(out, f)
	}
	return out, nil
}

// Strings is a list of text a call carried.
func Strings(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, it := range list {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Stamp is what a file looked like when last seen: a change to either is a change to send.
type Stamp struct {
	Size    int64
	ModTime int64
}

// Stamps is every regular file at the paths named under home, by its path under home, with its stamp: what Collect would send.
func Stamps(home string, abs ...string) map[string]Stamp {
	out := map[string]Stamp{}
	for _, p := range abs {
		filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() || isPart(path) {
				return nil // unreadable or gone: seen again at the next look
			}
			st, err := d.Info()
			if err != nil {
				return nil
			}
			if rel, err := Rel(home, path); err == nil {
				out[rel] = Stamp{Size: st.Size(), ModTime: st.ModTime().UnixNano()}
			}
			return nil
		})
	}
	return out
}

// Changed is the paths whose stamp differs in now from was, and those gone from now.
func Changed(was, now map[string]Stamp) (changed, gone []string) {
	for p, s := range now {
		if old, ok := was[p]; !ok || old != s {
			changed = append(changed, p)
		}
	}
	for p := range was {
		if _, ok := now[p]; !ok {
			gone = append(gone, p)
		}
	}
	return changed, gone
}
