package supervisor

import (
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/actor"
)

func init() { RunInline = true }

func (r *rig) say(in SayVerbIn) *SayVerbOut {
	r.t.Helper()
	tx, err := r.DB.Begin()
	r.must(err)
	out, err := r.SayVerb(tx, in)
	if err != nil {
		tx.Rollback()
		r.must(err)
	}
	r.must(tx.Commit())
	out.After()
	return out
}

func TestSayVerbQueuesAndTypesAtOnce(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	out := r.say(SayVerbIn{Requester: r.k(("alice")), Target: r.k(("bob")), Body: "psst"})
	if !out.OK || r.ids(out.Msg) != "queued as #1 for bob" {
		t.Fatalf("%+v", out)
	}
	if !strings.Contains(r.term.last(), "#1 from alice/lead, waiting for you:\npsst") {
		t.Fatalf("%q", r.term.last())
	}
	if out := r.say(SayVerbIn{Target: r.k(("bob")), Body: "x"}); out.OK || r.named(out.Msg) != noWho {
		t.Fatalf("%+v", out)
	}
}

// A reply to the founder's request needs no task named: work that follows it is a task begun or requested from the message.
func TestAReplyToTheFoundersRequestNeedsNoTask(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("alice"))
	ask := r.ask(r.k("human"), r.k(("alice")), "please build x", "")
	if out := r.say(SayVerbIn{Requester: r.k(("alice")), Target: r.k(("human")), Body: "on it", Parent: ask}); !out.OK {
		t.Fatalf("%+v", out)
	}
	var n int
	r.must(r.DB.QueryRow(r.sql("SELECT COUNT(*) FROM requests WHERE requester = 'alice' AND parent_id = ?"), ask).Scan(&n))
	if n != 1 {
		t.Fatalf("the reply is in the thread: %d", n)
	}
}

func TestPushVerbsCheckTheirTargetsAndHandOver(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	if ok, msg := r.PushVerb(r.k("human"), 0); ok || r.named(msg) != "human is not an agent" {
		t.Fatal(msg)
	}
	if ok, msg := r.PushVerb("nobody", 0); ok || r.named(msg) != "nobody is not an agent" {
		t.Fatal(msg)
	}
	rid := r.ask(r.k("alice"), r.k(("bob")), "x", "ask")
	if ok, msg := r.PushVerb(r.k("bob"), rid); !ok || msg != Handed {
		t.Fatal(msg)
	}
	if !strings.Contains(r.term.last(), "#1 from alice") {
		t.Fatalf("%q", r.term.last())
	}
	task := r.task(r.k("bob"), r.k(("alice")), "a task")
	r.term.typed = nil
	if ok, _ := r.PushVerb(r.k("bob"), task); !ok {
		t.Fatal("task push refused")
	}
}

func TestNotifyCancelledVerbOnlyForADeliveredRequestClosedUnderIt(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	rid := r.ask(r.k("alice"), r.k(("bob")), "job", "ask")
	if ok, msg, _ := r.NotifyCancelledVerb(r.DB, rid); ok || r.ids(msg) != "#1 was not cancelled after it was delivered" {
		t.Fatal(msg)
	}
	if ok, msg, _ := r.NotifyCancelledVerb(r.DB, 99); ok || r.named(msg) != "no such message" {
		t.Fatal(msg)
	}
	r.TellNow(r.DB, rid)
	after := &After{}
	plan, ok, _, err := r.CancelWrite(r.DB, after, rid, nil, nil)
	r.must(err)
	if !ok {
		t.Fatal("not cancelled")
	}
	r.term.typed = nil
	ok, msg, _ := r.NotifyCancelledVerb(r.DB, rid)
	if !ok || msg != Handed || !strings.Contains(r.term.last(), "#1 from alice/lead was cancelled: withdrawn by the founder") {
		t.Fatalf("%v %q %q", ok, msg, r.term.typed)
	}
	_ = plan
}

func TestASayToTheFounderThatNamesAMessageIsRefusedAndAsksForWords(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.say(SayVerbIn{Requester: r.k(("alice")), Target: r.k(("bob")), Body: "first"})
	var ref string
	r.DB.QueryRow(r.sql("SELECT ref FROM requests WHERE id = 1")).Scan(&ref)
	for _, body := range []string{"done, see msg_0123456", "as in " + ref[:8] + " earlier", "re " + strings.ToUpper(ref)} {
		out := r.say(SayVerbIn{Requester: r.k(("alice")), Target: actor.OwnerKey, Body: body})
		if out.OK || !strings.Contains(out.Msg, "say in words what that message was about") {
			t.Fatalf("%q: %+v", body, out)
		}
	}
	for _, body := range []string{"merged as 0123456789abcdef0123456789abcdef01234567", "PR #3 is up", "all green"} {
		if out := r.say(SayVerbIn{Requester: r.k(("alice")), Target: actor.OwnerKey, Body: body}); strings.Contains(out.Msg, "names a message") {
			t.Fatalf("%q is no message id: %+v", body, out)
		}
	}
}

// The daemon closes the database, and a test its home, only once a telling started off the caller has ended.
func TestWaitReturnsOnceTheTellingsOffTheCallerHaveEnded(t *testing.T) {
	RunInline = false
	t.Cleanup(func() { RunInline = true })
	e := &Env{}
	release, ended := make(chan struct{}), make(chan bool, 1)
	e.background("a telling", func() {
		<-release
		ended <- true
	})
	waited := make(chan struct{})
	go func() {
		e.Wait()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("Wait returned while the telling still ran")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-waited
	if len(ended) != 1 {
		t.Fatal("Wait returned before the telling ended")
	}
}
