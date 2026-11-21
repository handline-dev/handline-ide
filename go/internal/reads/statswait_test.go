package reads

import (
	"fmt"
	"testing"
	"time"
)

// stats.get's your_wait_s is the median of a person's turns from put on to given back, counting turns given back in the window;
// an agent's turn and a turn still open count for nothing.
func TestYourWaitIsTheMedianOfAPersonsTurns(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	now := time.Now().Unix()
	ev := func(ago int64, kind, payload string) {
		run(t, e, fmt.Sprintf("INSERT INTO events(ts, agent, kind, payload) VALUES(%d, 'x', '%s', '%s')", now-ago, kind, payload))
	}
	ev(9000, "task_turn_push", `{"task_id": 1, "to": "p1"}`)
	ev(8900, "task_turn_pop", `{"task_id": 1, "agent": "p1"}`) // 100s
	ev(5000, "task_turn_push", `{"task_id": 2, "to": "p1"}`)
	ev(4400, "task_turn_pop", `{"task_id": 2, "agent": "p1"}`) // 600s
	ev(3000, "task_turn_push", `{"task_id": 3, "to": "p1"}`)
	ev(2700, "task_turn_pop", `{"task_id": 3, "agent": "p1"}`) // 300s
	ev(2000, "task_turn_push", `{"task_id": 4, "to": "a3"}`)
	ev(100, "task_turn_pop", `{"task_id": 4, "agent": "a3"}`) // an agent's
	ev(500, "task_turn_push", `{"task_id": 5, "to": "p1"}`)   // still open
	got, err := e.statsGet(nil, map[string]any{"window": "24h"})
	if err != nil {
		t.Fatal(err)
	}
	if w := got.(map[string]any)["your_wait_s"]; w != int64(300) {
		t.Errorf("your_wait_s %v, want 300", w)
	}
}
