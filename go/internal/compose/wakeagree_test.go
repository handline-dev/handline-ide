package compose

import (
	"github.com/handline/handline/go/data/team/teamtest"
	"testing"

	dbpkg "github.com/handline/handline/go/data/db"
)

// The tick wakes a closed agent exactly when its wake's prompt has something to carry: a wake for nothing is refused and the agent stays
// down, and work the prompt could carry with no wake waits unseen.
func TestWhatWakesAClosedAgentIsWhatItsWakePromptCarries(t *testing.T) {
	type q = dbpkg.Q
	var alice string // the rig's alice, by key: each case sets it before its shape runs
	must := func(t *testing.T, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// task queues a task for alice from the founder; its request id.
	task := func(t *testing.T, d q, title string) (int64, int64) {
		tid, err := dbpkg.NewTask(d, title, alice, "p1", 0, dbpkg.DefaultPriority, "", 0, "")
		must(t, err)
		_, err = dbpkg.SetTaskQueued(d, tid, "p1")
		must(t, err)
		rid, err := dbpkg.AddRequest(d, dbpkg.NewRequest{Requester: "p1", Target: alice, Body: title, Kind: "task"})
		must(t, err)
		must(t, dbpkg.LinkRequest(d, tid, rid))
		return tid, rid
	}
	// held is a task alice was told and holds: its request has landed and stays at the head of her queue.
	held := func(t *testing.T, d q, title string) int64 {
		tid, rid := task(t, d, title)
		_, err := dbpkg.MarkDelivered(d, rid)
		must(t, err)
		must(t, dbpkg.MarkLanded(d, rid))
		return tid
	}
	blockOn := func(t *testing.T, d q, tid int64) {
		other, err := dbpkg.NewTask(d, "the founder's", "p1", "p1", 0, dbpkg.DefaultPriority, "", 0, "")
		must(t, err)
		_, err = d.Exec("INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(?, ?, 1)", tid, other)
		must(t, err)
	}
	say := func(t *testing.T, d q, body string) {
		_, err := dbpkg.AddRequest(d, dbpkg.NewRequest{Requester: "sys", Target: alice, Body: body, Kind: "say"})
		must(t, err)
	}
	const answered = "the founder's answer on a blocked task it holds"
	for name, shape := range map[string]func(*testing.T, q){
		// nothing to wake it for: its session has the task it holds
		"only the task it holds":         func(t *testing.T, d q) { held(t, d, "the one it holds") },
		"only the blocked task it holds": func(t *testing.T, d q) { blockOn(t, d, held(t, d, "blocked")) },
		"a new task":                     func(t *testing.T, d q) { task(t, d, "new work") },
		"a say":                          func(t *testing.T, d q) { say(t, d, "T9 is unblocked") },
		"a say behind the task it holds": func(t *testing.T, d q) { held(t, d, "the one it holds"); say(t, d, "T9 is unblocked") },
		"a task behind a blocked one it holds": func(t *testing.T, d q) {
			blockOn(t, d, held(t, d, "blocked"))
			task(t, d, "next")
		},
		"an unblocked task": func(t *testing.T, d q) {
			tid, _ := task(t, d, "was blocked")
			blockOn(t, d, tid)
			_, err := d.Exec("UPDATE task_blocks SET dormant = 1 WHERE task_id = ?", tid)
			must(t, err)
		},
		// T9080: the founder answered alice's question on a task that waits on another
		answered: func(t *testing.T, d q) {
			tid := held(t, d, "blocked")
			blockOn(t, d, tid)
			_, rid := task(t, d, "the answer")
			must(t, dbpkg.LinkRequest(d, tid, rid))
			_, err := dbpkg.AddEvent(d, "p1", "task_turn_pop", dbpkg.KV{K: "task_id", V: tid}, dbpkg.KV{K: "agent", V: "p1"}, dbpkg.KV{K: "request_id", V: rid})
			must(t, err)
		},
		// 10-19 17:28: the head held and blocked, a held task behind it, then the say that woke it
		"a say behind a blocked held task and a held one": func(t *testing.T, d q) {
			blockOn(t, d, held(t, d, "blocked"))
			held(t, d, "told at 17:20")
			say(t, d, "T4573 is unblocked")
		},
	} {
		t.Run(name, func(t *testing.T) {
			p, _, env, _ := wired(t)
			alice = teamtest.Key(t, p.home, "alice")
			d := p.db
			_, err := d.Exec("UPDATE requests SET state = 'answered' WHERE target = '" + alice + "'") // wired's own request
			must(t, err)
			shape(t, d)
			row, err := dbpkg.GetAgentState(d, alice)
			must(t, err)
			cause, err := p.Delivery().ClosedWakeCause(alice, row)
			must(t, err)
			_, _, ok, err := env.WaitingPrompt(d, alice)
			must(t, err)
			if (cause != "") != ok {
				t.Fatalf("the tick's cause %q, the prompt has something: %v", cause, ok)
			}
			if name == answered && cause == "" {
				t.Fatal("the founder's answer woke nothing")
			}
		})
	}
}
