package proposalwrites

import (
	"fmt"
	"testing"
)

func TestARunProposalNamesTheTaskItsAgentIsDoing(t *testing.T) {
	t.Parallel()
	r, _, dir := runRig(t)
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, sent_at) VALUES(7, 'w', '" + r.who("alice") + "', 'm', 1, 1), " +
			"(8, 'q', '" + r.who("bob") + "', 'm', 1, NULL)",
		"INSERT INTO agent_state(agent, state, updated, current_task) VALUES('" + r.who("alice") + "', 'IDLE', 1, 7), ('" + r.who("bob") + "', 'IDLE', 1, 8)",
	} {
		if _, err := r.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_, doing := r.proposeRun("alice", "alice", "echo a", dir)
	_, idle := r.proposeRun("bob", "bob", "echo b", dir) // it holds T8, queued: not doing it
	m := r.call("proposal.propose", fmt.Sprintf(`{"kind":"run","args":{"name":"alice","command":"echo c","cwd":%q},"why":"mine","request_id":"own"}`, dir))
	res, _ := m["result"].(map[string]any)
	n, _ := res["proposal"].(float64)
	own := fmt.Sprintf("%08x", int64(n))
	if doing == "" || idle == "" || res == nil {
		t.Fatalf("not raised: %v", m)
	}
	if got := r.e.ProposalView(doing)["task"]; got != int64(7) {
		t.Fatalf("alice's: task %v, want 7", got)
	}
	for _, pid := range []string{idle, own} {
		if v := r.e.ProposalView(pid); v == nil || v["task"] != nil {
			t.Fatalf("%s: %v, want no task", pid, v)
		}
	}
}
