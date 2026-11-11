package tick

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

type raised struct{ who, text []string }

func (r *raised) RaiseDecision(_ db.Q, who, text, _ string) error {
	r.who, r.text = append(r.who, who), append(r.text, text)
	return nil
}

// A wait-for cycle is raised to the agent acting on its task, not to the owner under that agent's turn.
func TestAWaitForCycleIsRaisedToTheAgentOnTop(t *testing.T) {
	tk, _ := newWorld(t)
	first, second := newTask(t, tk, "o", actor.OwnerKey, 0), newTask(t, tk, "o", actor.OwnerKey, 0)
	for _, tid := range []int64{first, second} {
		check(db.PushTurn(tk.Q, tid, db.Turn{Agent: "a", By: "o", Since: 1}))
	}
	check(db.AddBlock(tk.Q, first, second))
	check(db.AddBlock(tk.Q, second, first))
	r := &raised{}
	check((&WaitFor{Decide: r, Log: func(string) {}}).Sweep(tk.Q, 1000))
	if len(r.who) != 1 || r.who[0] != "a" || !strings.Contains(r.text[0], "T1 (a) -> T2 (a) -> T1 (a)") {
		t.Fatalf("%v %v", r.who, r.text)
	}
}
