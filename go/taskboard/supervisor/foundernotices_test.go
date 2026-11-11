package supervisor

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/peerinbox"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
)

func (r *rig) toFounder() int {
	r.t.Helper()
	var n int
	r.must(r.DB.QueryRow("SELECT COUNT(*) FROM requests WHERE target = ? AND requester = ?", db.Human, db.NoticeRequester).Scan(&n))
	return n
}

// A message of the founder's never seen to arrive is a warning kept under its request, in no line of their inbox.
func TestAFoundersLostMessageIsKeptAsAWarningAndSaysNothing(t *testing.T) {
	for _, written := range []bool{false, true} {
		r, in := peerRigHeld(t, peerinbox.Result{Kind: peerinbox.Delivered})
		rid := r.ask(db.Human, "bob", "psst", "say")
		op := requestOp(rid)
		if state, _ := r.sendOnce(r.DB, "bob", op, "psst", "now"); state != Written {
			t.Fatal(state)
		}
		eventually(t, func() bool { return in.enteredCount() == 1 })
		r.must(r.SessionNotDelivered("bob", op, "no inbox", written))
		raw, ok, err := db.Meta(r.DB, fmt.Sprintf("%s%d", db.LostMessagePrefix, rid))
		r.must(err)
		var u db.Undelivered
		if !ok || json.Unmarshal([]byte(raw), &u) != nil || u.Agent != "bob" || u.At == 0 ||
			strings.HasPrefix(u.Why, "was not written") == written {
			t.Fatalf("written=%v: %q", written, raw)
		}
		if n := r.toFounder(); n != 0 {
			t.Fatalf("written=%v: %d lines to the founder", written, n)
		}
	}
}

// Two of the founder's messages lost in one delivery each warn.
func TestEachOfTheFoundersMessagesLostInOneOpWarns(t *testing.T) {
	r := newRig(t)
	one, two := r.ask(db.Human, "bob", "one", "say"), r.ask(db.Human, "bob", "two", "say")
	r.deliveryUnknown(r.DB, "bob", requestOp(one, two), "was not read", "send it again")
	for _, rid := range []int64{one, two} {
		if _, ok, _ := db.Meta(r.DB, fmt.Sprintf("%s%d", db.LostMessagePrefix, rid)); !ok {
			t.Fatalf("no warning for %d", rid)
		}
	}
}

// A refused delivery with no lead to tell is a warning under the task; the board says nothing to the founder.
func TestARefusalWithNoLeadIsKeptAsAWarningAndSaysNothing(t *testing.T) {
	r := newRig(t)
	tid := r.newUnroutableTask()
	teamtest.Set(t, r.home, "eng.bob", "reports_to", nil) // no lead to tell: it reports to no agent
	r.DB.Exec("UPDATE workspaces SET lead = NULL WHERE name = 'eng'")
	if _, _, err := r.AdvanceCurrentTask(r.DB, "eng.bob"); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := db.Meta(r.DB, fmt.Sprintf("%s%d", db.RefusedTaskPrefix, tid))
	r.must(err)
	var u db.Undelivered
	if !ok || json.Unmarshal([]byte(raw), &u) != nil || u.Agent != "eng.bob" || u.Why == "" {
		t.Fatalf("%q", raw)
	}
	if n := r.toFounder(); n != 0 {
		t.Fatalf("%d lines to the founder", n)
	}
}

// When the last blocker of the founder's task is done, and its child failed, the board tells them neither; the task still comes free.
func TestADoneTellsAFounderHoldingTheNextTaskNothing(t *testing.T) {
	r := newRig(t)
	waiting := r.newTask(db.Human, "alice", "the founder's")
	child := r.newTask("alice", "alice", "child")
	r.must(db.AddBlock(r.DB, waiting, child))
	_, err := r.DB.Exec("UPDATE tasks SET parent_task = ?, done_evidence = 'FAILED: it does not build', done_at = 1 WHERE id = ?", waiting, child)
	r.must(err)
	notices, freed := r.freeBlockedBy(r.DB, child, "alice")
	if len(notices) != 0 || len(freed) != 1 || freed[0] != db.Human {
		t.Fatalf("%v %v", notices, freed)
	}
	if n := r.toFounder(); n != 0 {
		t.Fatalf("%d lines to the founder", n)
	}
}

// Whatever the board writes for a holder, a founder holding the task gets none of it, and the task still comes free.
func TestTheBoardWritesNothingToTheFounder(t *testing.T) {
	r := newRig(t)
	if rid, err := r.Notice(r.DB, db.Human, "a notice", "test"); err != nil || rid != 0 {
		t.Fatal(rid, err)
	}
	if rid := r.typeSay(r.DB, db.Human, "a say", "test"); rid != 0 {
		t.Fatal(rid)
	}
	blocker := r.newTask("alice", "alice", "blocker")
	child := r.newTask("alice", "alice", "child")
	waiting := r.newTask(db.Human, "alice", "the founder's")
	r.must(db.AddBlock(r.DB, waiting, blocker))
	r.must(db.AddBlock(r.DB, waiting, child))
	_, err := r.DB.Exec("UPDATE tasks SET parent_task = ? WHERE id = ?", waiting, child)
	r.must(err)
	_, err = db.DiscardTask(r.DB, child, "alice", "not needed")
	r.must(err)
	_, err = db.DiscardTask(r.DB, blocker, "alice", "not needed")
	r.must(err)
	if _, err := r.ReleaseEndedBlockers(); err != nil {
		t.Fatal(err)
	}
	if n := r.toFounder(); n != 0 {
		t.Fatalf("%d lines to the founder", n)
	}
	if left, _ := db.TaskBlockers(r.DB, waiting); len(left) != 0 {
		t.Fatalf("still blocked: %v", left)
	}
}
