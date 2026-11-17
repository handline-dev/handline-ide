// Package taskresult keeps what a task's end carries beyond its evidence: the commit it was done at and the screenshots that show it. The
// screenshots are copied under the daemon's home (shots/T<n>/<k>.png, outside any repo); the commit and the list are a meta record, so the
// database has no new column.
package taskresult

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/attachments"
)

const (
	MaxShots = 6
	MaxShot  = 10 << 20 // bytes of one screenshot
)

// Shot is one stored screenshot of a task, numbered from 1.
type Shot struct {
	N     int   `json:"n"`
	Bytes int64 `json:"bytes"`
}

// Result is what a finished task carries. Commit is the full sha it was done at, OnMain whether main reaches it.
type Result struct {
	Commit string `json:"commit,omitempty"`
	OnMain bool   `json:"on_main,omitempty"`
	Shots  []Shot `json:"shots,omitempty"`
}

// Empty is whether there is nothing to show.
func (r Result) Empty() bool {
	return r.Commit == "" && len(r.Shots) == 0
}

func resultKey(tid int64) string { return fmt.Sprintf("task-result:%d", tid) }
func uiKey(tid int64) string     { return fmt.Sprintf("task-ui:%d", tid) }

// Save keeps the result of task tid.
func Save(q db.Q, tid int64, r Result) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return db.SetMeta(q, resultKey(tid), string(b))
}

// Get is the result of task tid; the zero Result for a task with none.
func Get(q db.Q, tid int64) (Result, error) {
	var r Result
	v, ok, err := db.Meta(q, resultKey(tid))
	if err != nil || !ok {
		return r, err
	}
	if json.Unmarshal([]byte(v), &r) != nil {
		return Result{}, nil
	}
	return r, nil
}

// MarkUI records that task tid shows something on a screen.
func MarkUI(q db.Q, tid int64) error { return db.SetMeta(q, uiKey(tid), "1") }

// IsUI is whether task tid was opened as a ui task.
func IsUI(q db.Q, tid int64) (bool, error) {
	_, ok, err := db.Meta(q, uiKey(tid))
	return ok, err
}

// Dir is where the screenshots of task tid are kept.
func Dir(home string, tid int64) string { return filepath.Join(home, "shots", fmt.Sprintf("T%d", tid)) }

// ShotFile is where screenshot n of task tid is kept.
func ShotFile(home string, tid int64, n int) string {
	return filepath.Join(Dir(home, tid), fmt.Sprintf("%d.png", n))
}

// Stage reads the PNG files at paths and writes them to a staging folder beside the task's, answering what will be kept and the function
// that puts them in place. Nothing of an earlier result is touched until finalize runs, which the caller does once the task's end is
// committed: a done that fails leaves the shots it had. A path must be absolute, name a regular PNG file of at most MaxShot bytes and, when
// roots is not nil, sit under one of them (the file is read through the daemon's own handle, with links resolved first, so a link cannot
// lead it elsewhere).
func Stage(home string, tid int64, paths, roots []string) (shots []Shot, finalize func() error, err error) {
	if len(paths) > MaxShots {
		return nil, nil, bad(fmt.Sprintf("a task carries at most %d screenshots", MaxShots), "shots")
	}
	datas := make([][]byte, len(paths))
	for i, p := range paths {
		if datas[i], err = readPNG(p, roots); err != nil {
			return nil, nil, err
		}
	}
	dir := Dir(home, tid)
	stage := dir + ".stage"
	if err := os.RemoveAll(stage); err != nil {
		return nil, nil, err
	}
	if len(datas) == 0 {
		return nil, nil, nil
	}
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return nil, nil, err
	}
	shots = make([]Shot, len(datas))
	for i, b := range datas {
		if err := os.WriteFile(filepath.Join(stage, fmt.Sprintf("%d.png", i+1)), b, 0o600); err != nil {
			os.RemoveAll(stage)
			return nil, nil, err
		}
		shots[i] = Shot{N: i + 1, Bytes: int64(len(b))}
	}
	return shots, func() error {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		return os.Rename(stage, dir)
	}, nil
}

func bad(message, field string) *rpc.Error {
	return rpc.BadRequestError(message, field, rpc.InvalidParams)
}

func readPNG(p string, roots []string) ([]byte, error) {
	b, err := attachments.ReadUnder(p, roots, MaxShot, "screenshot", "shots")
	if err == nil && attachments.Sniff(b) != "image/png" {
		return nil, bad(p+" is not a PNG", "shots")
	}
	return b, err
}
