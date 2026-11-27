package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/agentctl/peerinbox"
	"github.com/handline/handline/go/agentctl/peerport"
	"github.com/handline/handline/go/data/db"
)

// fakeInbox is the peer inbox: it keeps what it was given and answers outcome.
type fakeInbox struct {
	mu      sync.Mutex
	texts   []string
	ids     []string // the message id each Deliver was given, as it begins
	outcome peerinbox.Result
	gate    chan struct{} // when set, a Deliver waits for it to close
	entered int
}

func (f *fakeInbox) Deliver(_ context.Context, _ peerinbox.Session, text string, _ peerinbox.From, id string) peerinbox.Result {
	f.mu.Lock()
	f.entered++
	f.ids = append(f.ids, id)
	f.mu.Unlock()
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texts = append(f.texts, text)
	return f.outcome
}

func (*fakeInbox) Watch(context.Context, peerinbox.Session, string, time.Time) (bool, bool) {
	return false, false
}

func (f *fakeInbox) Type(_ context.Context, _ peerinbox.Session, text string, typeIn func(string) error) peerinbox.Result {
	f.mu.Lock()
	f.entered++
	f.mu.Unlock()
	if err := typeIn(text); err != nil {
		return peerinbox.Result{Kind: peerinbox.NotDelivered, Why: err.Error()}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texts = append(f.texts, "typed: "+text)
	return f.outcome
}

func (f *fakeInbox) enteredCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.entered }

func (f *fakeInbox) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.texts) }

// peerRig is bob, a terminal agent delivered to through its peer inbox, under the real supervisor.
func peerRig(t *testing.T, outcome peerinbox.Result) (*rig, *fakeInbox) {
	r := newRig(t)
	// The agent_state row a running agent has: without it the session a request was told to is not kept, every tick reads the live
	// session as a new one, and the head is told again once its first telling has landed.
	if err := db.SetTerminal(r.DB, r.k(("bob")), "%bob", "/w/bob"); err != nil {
		t.Fatal(err)
	}
	in := &fakeInbox{outcome: outcome}
	var settled atomic.Int32
	// A delivery runs on its own goroutine and tells the database when it ends: the test does not end, and the database does not close,
	// before every delivery that began has told it.
	t.Cleanup(func() {
		for i := 0; i < 2000 && int(settled.Load()) < in.enteredCount(); i++ {
			time.Sleep(5 * time.Millisecond)
		}
	})
	r.Agents = &peerport.Port{
		Peer:   func(a string) bool { return a == r.k(("bob")) },
		Find:   func(string) (peerinbox.Session, error) { return peerinbox.Session{PID: 7}, nil },
		Sender: in,
		OnDelivered: func(a, op string) {
			defer settled.Add(1)
			if err := r.SessionConfirmed(r.DB, a, op); err != nil {
				t.Error(err)
			}
		},
		OnFailed: func(a, op, why string, written bool) {
			defer settled.Add(1)
			if err := r.SessionNotDelivered(a, op, why, written); err != nil {
				t.Error(err)
			}
		},
	}
	return r, in
}

// peerRigHeld is peerRig whose inbox does not answer until the test ends, for a test that calls sendOnce itself and settles the message by
// hand: the delivery sendOnce starts must not settle it first, nor outlive the database. Wait for in.enteredCount() before settling.
func peerRigHeld(t *testing.T, outcome peerinbox.Result) (*rig, *fakeInbox) {
	r, in := peerRig(t, outcome)
	in.gate = make(chan struct{})
	port := r.Agents.(*peerport.Port)
	settled := make(chan struct{}, 8)
	delivered, failed := port.OnDelivered, port.OnFailed
	port.OnDelivered = func(a, op string) { delivered(a, op); settled <- struct{}{} }
	port.OnFailed = func(a, op, why string, written bool) { failed(a, op, why, written); settled <- struct{}{} }
	t.Cleanup(func() {
		close(in.gate)
		for i := 0; i < in.enteredCount(); i++ {
			select {
			case <-settled:
			case <-time.After(10 * time.Second):
				return
			}
		}
	})
	return r, in
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("never happened")
}

func TestATerminalAgentIsToldThroughItsInboxAndTheTranscriptLandsIt(t *testing.T) {
	r, in := peerRig(t, peerinbox.Result{Kind: peerinbox.Delivered})
	rid := r.ask(r.k("alice"), r.k(("bob")), "psst", "say")
	if got, err := r.TellNow(r.DB, rid); err != nil || got != "sent" {
		t.Fatalf("%q %v", got, err)
	}
	eventually(t, func() bool { row, _ := db.GetRequest(r.DB, rid); return row.LandedAt.Valid })
	if in.count() != 1 || len(r.term.typed) != 0 {
		t.Fatalf("inbox %d, typed %v: nothing is typed at a terminal", in.count(), r.term.typed)
	}
	if again, _ := r.TellNow(r.DB, rid); again == "sent" || in.count() != 1 {
		t.Fatalf("a retry wrote again: %q", again)
	}
}

func TestATaskQueuedForATerminalAgentGoesThroughTheInboxOnce(t *testing.T) {
	r, in := peerRig(t, peerinbox.Result{Kind: peerinbox.Delivered})
	rid := r.ask(r.k("human"), r.k(("bob")), "ship it", "task")
	for i := 0; i < 2; i++ {
		if _, err := r.DeliverQueued(r.DB, r.k(("bob"))); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, func() bool { row, _ := db.GetRequest(r.DB, rid); return row.LandedAt.Valid })
	if in.count() != 1 {
		t.Fatalf("%d writes", in.count())
	}
}

func TestAMessageTheTranscriptNeverShowsNeedsAPersonAndIsNotWrittenAgain(t *testing.T) {
	r, in := peerRig(t, peerinbox.Result{Kind: peerinbox.NotDelivered, Why: "not in the receiver's transcript", Attempts: 2})
	rid := r.ask(r.k("alice"), r.k(("bob")), "psst", "say")
	if _, err := r.TellNow(r.DB, rid); err != nil {
		t.Fatal(err)
	}
	op := requestOp(rid)
	var told, needs int
	eventually(t, func() bool { // the sender is told last
		r.must(r.DB.QueryRow(r.sql("SELECT count(*) FROM requests WHERE target = 'alice' AND body LIKE '%transcript never showed it%'")).Scan(&told))
		return told > 0
	})
	r.must(r.DB.QueryRow(r.sql("SELECT count(*) FROM events WHERE agent = 'bob' AND kind = 'needs_you'")).Scan(&needs))
	if told != 1 || needs != 1 {
		t.Fatalf("sender told %d, needs-you events %d", told, needs)
	}
	if state, _ := r.sendOnce(r.DB, r.k(("bob")), op, "psst", delivery.Now); state != Unknown || in.count() != 1 {
		t.Fatalf("written again: %v %d", state, in.count())
	}
}

func TestATerminalAgentWithNoInboxIsQueuedNotTold(t *testing.T) {
	r, in := peerRig(t, peerinbox.Result{Kind: peerinbox.Delivered})
	r.Agents.(*peerport.Port).Find = func(string) (peerinbox.Session, error) { return peerinbox.Session{}, peerinbox.ErrNoSession }
	rid := r.ask(r.k("alice"), r.k(("bob")), "psst", "say")
	got, err := r.TellNow(r.DB, rid)
	if err != nil || got == "sent" || in.count() != 0 {
		t.Fatalf("%q %v %d", got, err, in.count())
	}
}

func TestAClosedTerminalAgentIsWokenForAMessageAtOnce(t *testing.T) {
	r, in := peerRig(t, peerinbox.Result{Kind: peerinbox.Delivered})
	r.Agents.(*peerport.Port).Find = func(string) (peerinbox.Session, error) { return peerinbox.Session{}, peerinbox.ErrNoSession }
	r.must(db.SetClosed(r.DB, r.k(("bob")), ""))
	rid := r.ask(r.k("alice"), r.k(("bob")), "psst", "say")
	if _, err := r.TellNow(r.DB, rid); err != nil || in.count() != 0 {
		t.Fatalf("%v %d", err, in.count())
	}
	if len(r.life.woke) != 1 || r.life.woke[0] != r.k(("bob")) {
		t.Fatalf("not left for the next tick: %q", r.life.woke)
	}
}

func TestAMessageNeverWrittenSaysSoAndOneWrittenAndUnseenSaysItMayHaveBeenRead(t *testing.T) {
	for _, written := range []bool{false, true} {
		r, in := peerRigHeld(t, peerinbox.Result{Kind: peerinbox.Delivered})
		rid := r.ask(r.k("alice"), r.k(("bob")), "psst", "say")
		op := requestOp(rid)
		if state, _ := r.sendOnce(r.DB, r.k(("bob")), op, "psst", delivery.Now); state != Written {
			t.Fatal(state)
		}
		eventually(t, func() bool { return in.enteredCount() == 1 })
		r.must(r.SessionNotDelivered(r.k("bob"), op, "no inbox", written))
		var n int
		pattern := "%was not written to its session (no inbox): it was not read. It is not sent again; send it again.%"
		if written {
			pattern = "%transcript never showed it (no inbox): it may or may not have been read. It is not sent again; ask " + r.k("bob") + " whether it saw it%"
		}
		r.must(r.DB.QueryRow(r.sql("SELECT count(*) FROM requests WHERE target = 'alice' AND body LIKE ?"), pattern).Scan(&n))
		if n != 1 {
			t.Fatalf("written=%v: the sender was told %d times", written, n)
		}
		if left, _ := db.PendingNotices(r.DB, r.k(("alice"))); len(left) != 1 || left[0].SendNow.Valid {
			t.Fatalf("written=%v: the sender's note waits in its digest, not typed at once: %+v", written, left)
		}
		r.must(r.SessionNotDelivered(r.k("bob"), op, "no inbox", written)) // settled once
		r.must(r.DB.QueryRow(r.sql("SELECT count(*) FROM events WHERE agent = 'bob' AND kind = 'needs_you'")).Scan(&n))
		if n != 1 {
			t.Fatalf("needs-you events: %d", n)
		}
	}
}

// callbacks lets a test end only once the port's callbacks in flight have returned; one that starts after is dropped.
type callbacks struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

func (c *callbacks) enter() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.wg.Add(1)
	}
	return !c.closed
}

func (c *callbacks) close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.wg.Wait()
}

// nextDaemon is the peer port a restarted daemon has: the real inbox watching bob's transcript in dir, through the session find gives.
// Its watch outlives the test, so the test's end waits out a callback that is running, before the rig puts the clock back.
func nextDaemon(r *rig, find func(string) (peerinbox.Session, error)) *peerport.Port {
	in := peerinbox.New()
	in.Poll, in.Now = 5*time.Millisecond, func() time.Time { return time.Unix(r.clock, 0) }
	ctx, cancel := context.WithCancel(context.Background())
	cb := &callbacks{}
	r.t.Cleanup(func() { cb.close(); cancel() })
	return &peerport.Port{
		Ctx:  ctx,
		Peer: func(a string) bool { return a == r.k("bob") }, Find: find, Sender: in,
		OnDelivered: func(a, op string) {
			if !cb.enter() {
				return
			}
			defer cb.wg.Done()
			if err := r.SessionConfirmed(r.DB, a, op); err != nil {
				r.t.Error(err)
			}
		},
		OnFailed: func(a, op, why string, written bool) {
			if !cb.enter() {
				return
			}
			defer cb.wg.Done()
			r.t.Errorf("%s: %s not delivered: %s", a, op, why)
		},
	}
}

// The daemon restarts after writing a message to bob and before bob's transcript shows it; bob's session runs on. The next daemon watches
// for it again, lands it once the line is there, and no one is told it may not have been read.
func TestAMessageWrittenBeforeARestartLandsWhenTheSameSessionShowsIt(t *testing.T) {
	r, in := peerRigHeld(t, peerinbox.Result{Kind: peerinbox.Delivered})
	rid := r.ask(r.k("alice"), r.k("bob"), "psst", "say")
	op := requestOp(rid)
	if state, _ := r.sendOnce(r.DB, r.k("bob"), op, "psst", delivery.Now); state != Written {
		t.Fatal(state)
	}
	eventually(t, func() bool { return in.enteredCount() == 1 })
	if in.ids[0] != peerport.MsgID(r.k("bob"), op) {
		t.Fatalf("written under %q: the next daemon cannot find it", in.ids[0])
	}
	dir := t.TempDir()
	transcript := filepath.Join(dir, "projects", "w", "s1.jsonl")
	r.must(os.MkdirAll(filepath.Dir(transcript), 0o755))
	r.must(os.WriteFile(transcript, []byte(`{"type":"user","message":{"content":"earlier"}}`+"\n"), 0o644))
	sess := peerinbox.Session{PID: 7, SessionID: "s1", Config: dir, StartedAt: (r.clock - 600) * 1000}
	next := nextDaemon(r, func(string) (peerinbox.Session, error) { return sess, nil })
	r.must(r.ResumeSends(r.DB, r.k("bob"), func(op string, written time.Time) bool { return next.Rewatch(r.k("bob"), op, written) }))
	if v, _ := db.Cursor(r.DB, ledgerKey(r.k("bob"), op)); v != sendWritten {
		t.Fatalf("the send is watched again, still written: ledger %d", v)
	}
	line := `{"type":"user","timestamp":"` + time.Unix(r.clock, 0).UTC().Format(time.RFC3339) + `","origin":{"kind":"peer"},` +
		`"message":{"content":"psst\n[handline-msg ` + peerport.MsgID(r.k("bob"), op) + `]"}}` + "\n"
	f, err := os.OpenFile(transcript, os.O_APPEND|os.O_WRONLY, 0)
	r.must(err)
	_, err = f.WriteString(line)
	r.must(err)
	r.must(f.Close())
	eventually(t, func() bool { row, _ := db.GetRequest(r.DB, rid); return row.LandedAt.Valid })
	var told int
	r.must(r.DB.QueryRow(r.sql("SELECT count(*) FROM requests WHERE target = 'alice' AND body LIKE '%may or may not%'")).Scan(&told))
	if told != 0 {
		t.Fatalf("the sender was told %d times that it may not have been read", told)
	}
}

// A session that did not outlive the restart, gone or started after the write, cannot show the message: it is unknown at once and the
// sender is told.
func TestAMessageWrittenBeforeARestartIsUnknownWhenItsSessionEnded(t *testing.T) {
	for name, find := range map[string]func(string) (peerinbox.Session, error){
		"gone": func(string) (peerinbox.Session, error) { return peerinbox.Session{}, peerinbox.ErrNoSession },
		"newer": func(string) (peerinbox.Session, error) {
			return peerinbox.Session{PID: 8, SessionID: "s2", StartedAt: 100001 * 1000}, nil
		},
	} {
		r, in := peerRigHeld(t, peerinbox.Result{Kind: peerinbox.Delivered})
		rid := r.ask(r.k("alice"), r.k("bob"), "psst", "say")
		op := requestOp(rid)
		if state, _ := r.sendOnce(r.DB, r.k("bob"), op, "psst", delivery.Now); state != Written {
			t.Fatal(state)
		}
		eventually(t, func() bool { return in.enteredCount() == 1 })
		next := nextDaemon(r, find)
		r.must(r.ResumeSends(r.DB, r.k("bob"), func(op string, written time.Time) bool { return next.Rewatch(r.k("bob"), op, written) }))
		if v, _ := db.Cursor(r.DB, ledgerKey(r.k("bob"), op)); v != sendUnknown {
			t.Fatalf("%s: ledger %d", name, v)
		}
		var told int
		r.must(r.DB.QueryRow(r.sql("SELECT count(*) FROM requests WHERE target = 'alice' AND body LIKE '%session ended before it confirmed it%'")).Scan(&told))
		if told != 1 {
			t.Fatalf("%s: the sender was told %d times", name, told)
		}
	}
}

func TestAMessageTheLastDaemonLeftWrittenIsUnknownWhenTheNextSettlesIt(t *testing.T) {
	r, in := peerRigHeld(t, peerinbox.Result{Kind: peerinbox.Delivered})
	rid := r.ask(r.k("alice"), r.k(("bob")), "psst", "say")
	op := requestOp(rid)
	if state, _ := r.sendOnce(r.DB, r.k(("bob")), op, "psst", delivery.Now); state != Written {
		t.Fatal(state)
	}
	eventually(t, func() bool { return in.enteredCount() == 1 })
	r.must(r.ResumeSends(r.DB, r.k("bob"), nil)) // what the daemon does at its start for each peer agent
	if v, _ := db.Cursor(r.DB, ledgerKey(r.k("bob"), op)); v != sendUnknown {
		t.Fatalf("ledger %d", v)
	}
}
