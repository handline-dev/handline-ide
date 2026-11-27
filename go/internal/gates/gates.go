// Package gates opens the gate tasks: a task owned by db.Gate watches a vendor's target (package watch), and is marked done with the
// vendor's evidence when the vendor answers passed or failed, or when its deadline passes. The tasks it blocks then come back to their
// holders by the ordinary unblock; what follows is theirs to decide, unless the gate was asked to close its task when it passes
// (land.go). No vendor code is here.
package gates

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/taskboard/taskwrites"
	"github.com/handline/handline/go/taskboard/watch"
)

// EveryS is how often the gates are looked at (Run); ReadWait is how long one read of a vendor may take.
const (
	EveryS   = 60
	ReadWait = 20 * time.Second
)

// Sweep is the gate pass. It runs on its own clock (Run), off the tick, so a slow read of a vendor never holds up a wake or a delivery.
type Sweep struct {
	DB   *sql.DB
	Home string
	Tell taskwrites.Teller
	Log  func(format string, args ...any)
	// Vendors read the targets; nil is watch.Vendors.
	Vendors map[string]watch.Vendor
	// Fetch brings origin's default branch of a roster folder up to date before a merge commit is looked for on it; nil is FetchBase.
	Fetch    func(repo string) error
	gone     map[int64]int    // gone answers in a row, by gate
	unlanded map[int64]int    // looks in a row that fetched and did not find the merge commit, by gate
	unread   map[int64]string // why a task's pull request's comments could not be read, as last logged, by task
}

// Run passes every EveryS until stop closes; stop also cuts short a read in hand.
func (s *Sweep) Run(stop <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	t := time.NewTicker(EveryS * time.Second)
	defer t.Stop()
	for {
		if err := s.Pass(ctx, time.Now().Unix()); err != nil {
			s.logf("  gates: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Pass opens every gate whose vendor answered passed or failed, that read gone watch.GoneReads times in a row, or whose deadline passed. A
// read that fails keeps the gate watching, with the reason on it. A gate that closes a task opens with that close, or stays open while
// the close cannot be checked. Every open gate is looked at on every pass: what a pass could not finish, the next one does. Then the
// comments on open tasks' pull requests are passed on (relayComments).
func (s *Sweep) Pass(ctx context.Context, now int64) error {
	ids, watches, deadlines, err := db.OpenGates(s.DB)
	if err != nil {
		return err
	}
	vendors := s.Vendors
	if vendors == nil {
		vendors = watch.Vendors
	}
	if s.gone == nil {
		s.gone, s.unlanded = map[int64]int{}, map[int64]int{}
	}
	for i, tid := range ids {
		if ctx.Err() != nil {
			return nil
		}
		w := watches[i]
		var evidence, result string
		var c closing // what a passed gate does about the task it closes; zero for a gate that closes none
		if deadlines[i] != 0 && now >= deadlines[i] {
			evidence, result, c = s.pastDeadline(tid, w)
		} else {
			why := ""
			evidence, result, why = s.read(ctx, vendors[w.Vendor], tid, w)
			if result == string(watch.Passed) && w.Closes != 0 {
				if c, err = s.closingOf(tid, w, evidence); err != nil {
					s.logf("  T%d: %v", tid, err)
					continue
				}
				if c.wait != "" { // passed, and the close cannot be checked yet: the gate stays open and is read again
					result, why = "", c.wait
				}
			}
			// A read that fails after the merge was seen does not forget it: the gate still opens with it at its deadline.
			if _, _, isHeld := heldPass(w); isHeld && result == "" && !strings.HasPrefix(why, db.PassHeld) {
				why = w.Error
			}
			if why != w.Error {
				if err := db.SetWatchError(s.DB, tid, why); err != nil {
					s.logf("  T%d: %v", tid, err)
				}
			}
		}
		if result == "" {
			continue
		}
		delete(s.gone, tid)
		delete(s.unlanded, tid)
		if c.task != 0 {
			err = s.openClosing(tid, evidence, c)
		} else {
			err = s.open(tid, evidence, result)
		}
		if err != nil {
			s.logf("  T%d: the gate could not be opened: %v", tid, err)
			continue
		}
		s.logf("  T%d: gate on %s:%s %s opened - %s", tid, w.Vendor, w.Target, w.Until, evidence)
	}
	return s.relayComments(ctx, vendors) // what people wrote on the tasks' pull requests, read on the same clock
}

// read asks the gate's vendor once: the evidence and result it opens with ("" while it watches), and why the read did not answer.
func (s *Sweep) read(ctx context.Context, v watch.Vendor, tid int64, w db.Watch) (evidence, result, why string) {
	if v == nil {
		return "", "", "no vendor " + w.Vendor + " in this daemon"
	}
	rctx, cancel := context.WithTimeout(ctx, ReadWait)
	verdict, evidence, err := v.Check(rctx, w.Target, w.Until, time.Unix(w.Since, 0))
	cancel()
	if verdict != watch.Gone {
		delete(s.gone, tid)
	}
	switch {
	case err != nil:
		s.logf("  T%d: %s:%s could not be read: %v", tid, w.Vendor, w.Target, err)
		return "", "", err.Error()
	case verdict == watch.Gone:
		if s.gone[tid]++; s.gone[tid] < watch.GoneReads {
			return "", "", "gone: " + evidence
		}
		return "gone: " + evidence, string(watch.Failed), ""
	case verdict == watch.Passed || verdict == watch.Failed:
		return evidence, string(verdict), ""
	}
	return "", "", ""
}

// open marks gate tid done with evidence, as its owner, and keeps the result; the tasks it blocks are told after the commit.
func (s *Sweep) open(tid int64, evidence, result string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	done, err := taskwrites.MarkDone(tx, s.Home, db.Gate, taskwrites.DoneIn{Task: tid, Words: evidence})
	if err != nil {
		return err
	}
	if err := db.SetWatchResult(tx, tid, result); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.Tell != nil {
		s.Tell.Done(tid, db.Gate, done.Owner, done.Told, done.Finished)
	}
	return nil
}

func (s *Sweep) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}
