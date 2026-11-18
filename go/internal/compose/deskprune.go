package compose

import (
	"errors"
	"path/filepath"
	"strconv"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/worktreeprune"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/scheduler/sysres"
	"github.com/handline/handline/go/scheduler/worktreepool"
)

// pruneCursor is the local day (yyyymmdd) the nightly prune last ran; diskEvery how often the disk target is checked, in seconds.
const (
	pruneCursor = "desks_pruned_day"
	diskEvery   = 600
)

// deskPrune removes desks: once a night, each project's free desks beyond the number it keeps that went unused for
// worktreepool.UnusedS; and when the founder set disk_keep_free_gb, free desks of any project, oldest first, while free disk is under it.
type deskPrune struct {
	pool *worktreepool.Pool
	q    db.Q
	home string
	free func(path string) (free, total int64, err error)
	log  func(string, ...any)

	diskChecked int64
	toldShort   bool // "nothing left to remove" was logged; again only once free disk is back over the target
}

func (d *deskPrune) Run(now int64) error {
	if d.pool == nil {
		return nil
	}
	return errors.Join(d.nightly(now), d.disk(now))
}

func (d *deskPrune) nightly(now int64) error {
	t := time.Unix(now, 0)
	day := int64(t.Year()*10000 + int(t.Month())*100 + t.Day())
	if t.Hour() < worktreepool.PruneHour {
		return nil
	}
	if last, err := db.Cursor(d.q, pruneCursor); err != nil || last == day {
		return err
	}
	folders, err := worktreepool.List(d.q, "")
	if err != nil {
		return err
	}
	seen, st := map[string]bool{}, protected.New(d.home)
	var errs []error
	for _, f := range folders {
		if seen[f.Repo] {
			continue
		}
		seen[f.Repo] = true
		keep, ok, err := worktreepool.KeptNumber(d.q, st, f.Repo, t)
		if err != nil || !ok {
			errs = append(errs, err)
			continue
		}
		pruned, err := d.pool.Prune(f.Repo, keep, now-worktreepool.UnusedS)
		d.said(pruned)
		errs = append(errs, err)
	}
	return errors.Join(append(errs, db.SetCursor(d.q, pruneCursor, day))...) // a failed project waits for the next night
}

func (d *deskPrune) disk(now int64) error {
	if now-d.diskChecked < diskEvery {
		return nil
	}
	d.diskChecked = now
	target := worktreepool.GB(usage.LoadConfig(d.home), worktreepool.DiskKeepFreeKey)
	if target == 0 {
		return nil
	}
	free := d.free
	if free == nil {
		free = sysres.StatfsFree
	}
	freeNow := func() int64 {
		f, _, err := free(d.home)
		if err != nil {
			return target // unreadable: nothing is removed on it
		}
		return f
	}
	gap := target - freeNow()
	if gap <= 0 {
		d.toldShort = false
		return nil
	}
	size := func(path string) int64 {
		v, ok, err := db.Meta(d.q, worktreeprune.SizePrefix+path)
		if err != nil || !ok {
			return 0
		}
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	pruned, err := d.pool.Free(func() bool { return freeNow() < target }, gap, size)
	d.said(pruned)
	if err == nil && len(pruned) == 0 && !d.toldShort {
		d.toldShort = true
		d.log("  desks: free disk is under its target of %d GB and no free desk can be removed", target>>30)
	}
	return err
}

func (d *deskPrune) said(pruned []worktreepool.Pruned) {
	for _, p := range pruned {
		d.log("  desks: removed %s (%s): %s", filepath.Base(p.Path), filepath.Dir(p.Path), p.Why)
	}
}
