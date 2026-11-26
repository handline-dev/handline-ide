// Package boardcopy is the copy of the board the owner's machines keep (docs/several-machines.md section 4): Litestream, as a
// library inside the master, writes each committed change of the board to a folder on the master (Copier); each of the owner's runners
// pulls what it lacks of that folder in pieces (Changes on the master, Apply on the runner); a copy restores to a board file (Restore).
package boardcopy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/benbjohnson/litestream"
	"github.com/benbjohnson/litestream/file"
	"github.com/superfly/ltx"
)

// Dir is where a home keeps the board's copy: the master's replica, a runner's mirror of it.
func Dir(home string) string { return filepath.Join(home, "board-copy") }

// Snapshot and Keep bound the folder: a full snapshot this often, and what is older than Keep dropped, so at most three snapshots of
// the board are ever kept.
const (
	Snapshot = time.Hour
	Keep     = 2 * time.Hour
)

// marker holds the id of the board a folder is the copy of (kept in the board itself, so a board made anew has another): a folder
// made for another board (a runner's mirror, a -db elsewhere, a board set aside) is cleared before Litestream writes into it. It is
// the master's own and never sent.
const marker = ".board"

// Copier is Litestream replicating the board file to Dir on the master.
type Copier struct{ store *litestream.Store }

// Start starts replicating board (the database file) to dir; id is the board's own id (BoardID). Stop it after the board's own
// connections have closed.
func Start(board, dir, id string) (*Copier, error) {
	if b, err := os.ReadFile(filepath.Join(dir, marker)); err != nil || string(b) != id {
		if err := os.RemoveAll(dir); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, marker), []byte(id), 0o600); err != nil {
		return nil, err
	}
	quiet := slog.New(slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{Level: slog.LevelWarn}))
	db := litestream.NewDB(board)
	db.Logger = quiet
	client := file.NewReplicaClient(dir)
	rep := litestream.NewReplicaWithClient(db, client)
	db.Replica, client.Replica = rep, rep
	store := litestream.NewStore([]*litestream.DB{db}, litestream.CompactionLevels{{Level: 0}, {Level: 1, Interval: time.Minute}})
	store.SnapshotInterval, store.SnapshotRetention, store.Logger = Snapshot, Keep, quiet
	if err := store.Open(context.Background()); err != nil {
		return nil, fmt.Errorf("board copy: %w", err)
	}
	return &Copier{store: store}, nil
}

// Synced writes every change committed to the board so far into the copy's folder, and answers the last transaction the folder then
// holds: a mirror whose files reach it (Upto) holds every one of those changes.
func (c *Copier) Synced(ctx context.Context) (uint64, error) {
	var last uint64
	for _, db := range c.store.DBs() {
		if err := db.SyncAndWait(ctx); err != nil {
			return 0, fmt.Errorf("board copy: %w", err)
		}
		pos, err := db.Pos()
		if err != nil {
			return 0, fmt.Errorf("board copy: %w", err)
		}
		last = max(last, uint64(pos.TXID))
	}
	return last, nil
}

// Upto is the last transaction a copy holding files has: the highest any of its files ends at. A mirror pulls every file and drops
// one only once what replaces it has come, so the files before it are there too.
func Upto(files []File) uint64 {
	var last uint64
	for _, f := range files {
		if _, end, err := ltx.ParseFilename(path.Base(f.Path)); err == nil {
			last = max(last, uint64(end))
		}
	}
	return last
}

// Stop stops replicating.
func (c *Copier) Stop() error { return c.store.Close(context.Background()) }

// File is one file of a copy by its path under the folder and its size; a name Litestream writes is never written again, so the two
// say whether a mirror holds it.
type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Piece is part of a file a pull sends: Data at Offset of the file Path of Size bytes; the last piece carries the whole file's Sum.
type Piece struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Offset int64  `json:"offset"`
	Data   []byte `json:"data"`
	Sum    string `json:"sum,omitempty"`
}

// List is every whole file under dir, by path: none that a writer has not renamed in yet (.tmp, a mirror's .part).
func List(dir string) ([]File, error) {
	var out []File
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.Type().IsRegular() || strings.HasSuffix(p, ".tmp") || strings.HasSuffix(p, partSuffix) || d.Name() == marker {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) { // compacted away meanwhile
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, File{Path: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// Changes is what a mirror holding have (whole files) and parts (files it has the first bytes of) lacks of dir, up to limit bytes in
// all: the pieces to send, and once nothing is left to send, the paths it holds that dir no longer has. more is what is left for the
// next pull; gone comes only once every piece has been sent, so a mirror never drops a file before what replaces it has arrived.
func Changes(dir string, have, parts []File, limit int64) (send []Piece, gone []string, more bool, err error) {
	files, err := List(dir)
	if err != nil {
		return nil, nil, false, err
	}
	held, part := map[string]int64{}, map[string]int64{}
	for _, f := range have {
		held[f.Path] = f.Size
	}
	for _, f := range parts {
		part[f.Path] = f.Size
	}
	var size int64
	for _, f := range files {
		if s, ok := held[f.Path]; ok && s == f.Size {
			delete(held, f.Path)
			continue
		}
		delete(held, f.Path)
		if size >= limit {
			more = true
			continue
		}
		off := part[f.Path]
		if off < 0 || off > f.Size {
			off = 0
		}
		p, err := piece(dir, f, off, limit-size)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, false, err
		}
		size += int64(len(p.Data)) + entryCost
		send = append(send, p)
		if p.Offset+int64(len(p.Data)) < f.Size {
			more = true
		}
	}
	if !more {
		for p := range held {
			gone = append(gone, p)
		}
		sort.Strings(gone)
		if n := max(int((limit-size)/entryCost), 1); len(gone) > n { // the rest in the next answer: nothing is left to send by then
			gone, more = gone[:n], true
		}
	}
	return send, gone, more, nil
}

// entryCost is what an answer spends on one piece or one gone path beyond its data, counted against the answer's bytes.
const entryCost = 256

// piece reads up to n bytes of f from off, with the file's sum when they reach its end.
func piece(dir string, f File, off, n int64) (Piece, error) {
	h, err := os.Open(filepath.Join(dir, filepath.FromSlash(f.Path)))
	if err != nil {
		return Piece{}, err
	}
	defer h.Close()
	data := make([]byte, min(n, f.Size-off))
	if _, err := h.ReadAt(data, off); err != nil && !errors.Is(err, io.EOF) {
		return Piece{}, err
	}
	p := Piece{Path: f.Path, Size: f.Size, Offset: off, Data: data}
	if off+int64(len(data)) == f.Size {
		sum := sha256.New()
		if _, err := h.Seek(0, io.SeekStart); err != nil {
			return Piece{}, err
		}
		if _, err := io.Copy(sum, h); err != nil {
			return Piece{}, err
		}
		p.Sum = hex.EncodeToString(sum.Sum(nil))
	}
	return p, nil
}

// partSuffix marks a file a mirror is still receiving.
const partSuffix = ".part"

// Parts is the files dir is still receiving, each with the bytes it has.
func Parts(dir string) ([]File, error) {
	var out []File
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(p, partSuffix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, strings.TrimSuffix(p, partSuffix))
		if err != nil {
			return err
		}
		out = append(out, File{Path: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	return out, err
}

// Apply writes what a pull sent into dir: each piece into the file's .part at its offset, and a file whose last piece came, checked
// against its sum, synced and renamed in, so no partial file is ever under its own name; then the gone ones are removed. A path that
// leaves dir is refused.
func Apply(dir string, send []Piece, gone []string) error {
	for _, p := range send {
		dest, err := inside(dir, p.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		flags := os.O_RDWR | os.O_CREATE
		if p.Offset == 0 {
			flags |= os.O_TRUNC
		}
		f, err := os.OpenFile(dest+partSuffix, flags, 0o600)
		if err != nil {
			return err
		}
		_, err = f.WriteAt(p.Data, p.Offset)
		if err == nil && p.Sum != "" {
			err = finish(f, dest, p)
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dest + partSuffix) // sent again whole on the next pull
			return err
		}
		if p.Sum != "" {
			if err := os.Rename(dest+partSuffix, dest); err != nil {
				return err
			}
		}
	}
	for _, g := range gone {
		p, err := inside(dir, g)
		if err != nil {
			return err
		}
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// finish checks the received file against the sum its last piece carried and syncs it, before it is renamed in.
func finish(f *os.File, dest string, p Piece) error {
	if err := f.Truncate(p.Size); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != p.Sum {
		return fmt.Errorf("board copy: %s arrived otherwise than sent", p.Path)
	}
	return f.Sync()
}

func inside(dir, rel string) (string, error) {
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if r, err := filepath.Rel(dir, p); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("board copy: %q is not inside the copy", rel)
	}
	return p, nil
}

// Restore makes a board file at dest from the copy in dir.
func Restore(dir, dest string) error {
	client := file.NewReplicaClient(dir)
	rep := litestream.NewReplicaWithClient(nil, client)
	client.Replica = rep
	opt := litestream.NewRestoreOptions()
	opt.OutputPath = dest
	return rep.Restore(context.Background(), opt)
}

// DropParts removes the files dir was receiving that are not among sent: their sources are gone from the copy, so they never finish.
func DropParts(dir string, sent []Piece) error {
	parts, err := Parts(dir)
	if err != nil {
		return err
	}
	still := map[string]bool{}
	for _, p := range sent {
		still[p.Path] = true
	}
	for _, f := range parts {
		if !still[f.Path] {
			p, err := inside(dir, f.Path)
			if err != nil {
				return err
			}
			if err := os.Remove(p + partSuffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
