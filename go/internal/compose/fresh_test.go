package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// The delivery asks the cost pilot, with config.yaml's switch, whether bob's next task waits for a fresh session.
func TestTheDeliveryHoldsANextTaskForTheFreshSessionWhenTheGlobalSwitchIsOn(t *testing.T) {
	p, home := rig(t)
	lc := p.Lifecycle(cfgOf(p), LifecycleConfig{Env: &supervisor.Env{DB: p.db, Home: home, Log: func(string, ...any) {}}})
	db.SetAgentState(p.db, "bob", states.Idle, 0, "")
	p.db.Exec("UPDATE agent_state SET terminal = 'pane' WHERE agent = 'bob'")
	p.db.Exec("INSERT INTO tasks(title, owner, opened_by, created_at, done_evidence, done_at) VALUES('done', 'bob', 'lead', 1, 'ok', 100)")
	tid, _ := db.NewTask(p.db, "next", "bob", "lead", 0, db.DefaultPriority, "", 0, "")
	rid, _ := db.AddRequest(p.db, db.NewRequest{Requester: "lead", Target: "bob", Body: "do it", Kind: "task"})
	db.LinkRequest(p.db, tid, rid)
	for _, on := range []bool{false, true} {
		yaml := "fresh_after_task: false\n"
		if on {
			yaml = "fresh_after_task: true\n"
		}
		os.WriteFile(filepath.Join(home, "config.yaml"), []byte(yaml), 0o644)
		if held, err := lc.Env.FreshDue(p.db, "bob"); held != on || err != nil {
			t.Fatalf("switch %v: held %v, %v", on, held, err)
		}
	}
}
