package supervisor

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

func TestANoticeIsToldToTheOpenerAndTheFounderHearsOnceIfTheTaskDoesNotMoveInAnHour(t *testing.T) {
	r := newRig(t)
	tid := r.newTask(r.k("bob"), r.k("alice"), "the job")
	r.must(r.TellAbout(r.DB, r.k("alice"), r.k("bob"), tid, "bob looks stuck", "test", r.clock))
	var to string
	r.DB.QueryRow("SELECT target FROM requests WHERE body = 'bob looks stuck'").Scan(&to)
	if to != r.k("alice") {
		t.Fatalf("the opener is told: %q", to)
	}
	var linked int
	r.DB.QueryRow("SELECT COUNT(*) FROM task_requests l JOIN requests q ON q.id = l.request_id WHERE l.task_id = ? AND q.body = 'bob looks stuck'", tid).Scan(&linked)
	if linked != 1 {
		t.Fatalf("the notice is linked to its task: %d", linked)
	}
	founder := func() (string, bool) {
		raw, ok, err := db.Meta(r.DB, fmt.Sprintf("%sT%d", db.FounderNoticePrefix, tid))
		r.must(err)
		return raw, ok
	}
	r.must(r.SweepNoticeWatches(r.clock + db.NoticeWatchS - 1))
	if _, ok := founder(); ok {
		t.Fatal("told the founder before the hour")
	}
	r.must(r.SweepNotices(r.clock + db.NoticeWatchS)) // the tick's notice sweep runs it
	raw, ok := founder()
	var n db.FounderNotice
	if !ok || json.Unmarshal([]byte(raw), &n) != nil || n.Agent != r.k("alice") || n.Task != tid ||
		n.Text != fmt.Sprintf("T%d: told <@%s>, no movement in 1h: bob looks stuck", tid, r.k("alice")) {
		t.Fatalf("the founder hears once alice let it sit: %q", raw)
	}
	r.must(db.ClearMeta(r.DB, fmt.Sprintf("%sT%d", db.FounderNoticePrefix, tid)))
	r.must(r.SweepNoticeWatches(r.clock + 2*db.NoticeWatchS))
	if _, ok := founder(); ok {
		t.Fatal("the founder is told once")
	}
	// a notice the founder is to get, or one for an agent that is not registered, is a warning, not a message
	r.must(r.TellAbout(r.DB, actor.OwnerKey, r.k("bob"), tid, "for the founder", "test", r.clock))
	r.must(r.TellAbout(r.DB, "nobody", r.k("bob"), 0, "no one to tell", "test", r.clock))
	var sent int
	r.DB.QueryRow("SELECT COUNT(*) FROM requests WHERE body IN ('for the founder', 'no one to tell')").Scan(&sent)
	if _, okBob, _ := db.Meta(r.DB, db.FounderNoticePrefix+r.k("bob")+":test"); sent != 0 || !okBob {
		t.Fatalf("%d sent, warning kept %v", sent, okBob)
	}
	// once bob acts the warning is over, and the sweep drops it
	_, err := r.DB.Exec("INSERT INTO requests(ts, requester, target, body, state) VALUES(?, ?, ?, 'back', 'landed')", r.clock+1, r.k("bob"), r.k("alice"))
	r.must(err)
	r.must(r.SweepNoticeWatches(r.clock + 2))
	if _, okBob, _ := db.Meta(r.DB, db.FounderNoticePrefix+r.k("bob")+":test"); okBob {
		t.Fatal("an over warning is kept")
	}
}
