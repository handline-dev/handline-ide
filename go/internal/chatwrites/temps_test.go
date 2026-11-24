package chatwrites

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/team"
)

// seat is a new seat of agent, at work on tid (0: idle).
func (r *rig) seat(agent string, tid int64) string {
	r.t.Helper()
	seat, err := team.NewSeat(r.db, agent, 1)
	if err == nil {
		err = team.SetHiredFor(r.db, seat, tid)
	}
	if err != nil {
		r.t.Fatal(err)
	}
	return seat
}

// A message to a seat at work reaches it; to an idle seat, the agent it is of.
func TestAWordToAnIdleSeatReachesItsAgent(t *testing.T) {
	r := newRig(t)
	alice := r.k("alice")
	busy, idle := r.seat(alice, 99), r.seat(alice, 0)
	founder := r.server("")
	for i, c := range []struct{ to, want string }{{busy, busy}, {idle, alice}} {
		res := r.result(r.one(founder, "chat.send", fmt.Sprintf(`{"agent":%q,"body":"hi","request_id":"s%d"}`, c.to, i)))
		var target string
		r.db.QueryRow("SELECT target FROM requests WHERE id = ?", int64(res["request"].(float64))).Scan(&target)
		if target != c.want {
			t.Fatalf("a message to %s went to %s, want %s", c.to, target, c.want)
		}
	}
}
