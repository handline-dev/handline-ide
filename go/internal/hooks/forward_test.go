package hooks

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/protocol/rpc"
)

// On a runner each event goes to the master, its own tracker hearing it first; while the master cannot be reached the event waits in
// the spool and goes, in order and once, when it can; one the master refuses is not sent again.
func TestARunnersHooksGoToTheMasterOrWaitInTheSpool(t *testing.T) {
	home := t.TempDir()
	type sent struct{ kind, rid string }
	var got []sent
	reachable, refuse := false, ""
	tracker := &turns.Tracker{}
	h := &Recorder{Home: home, Log: t.Logf, Turns: tracker, Forward: func(agent, kind string, _ map[string]any, rid string, _ int64) (string, error) {
		if !reachable {
			return "", ErrUnreached
		}
		if kind == refuse {
			return "", rpc.Refused("no", "not_on_node", "")
		}
		got = append(got, sent{kind, rid})
		return "keep going", nil
	}}
	record := func(kind, rid string, at int64) map[string]any {
		t.Helper()
		out, err := h.Record(&rpc.Conn{}, map[string]any{"agent": "bob", "kind": kind, "data": map[string]any{},
			"at": json.Number(strconv.FormatInt(at, 10)), "request_id": rid})
		if err != nil {
			t.Fatal(err)
		}
		return out.(map[string]any)
	}
	record("UserPromptSubmit", "r1", 1)
	if tracker.State("bob") != turns.Working {
		t.Fatal("the runner's tracker did not hear a spooled event")
	}
	record("Stop", "r2", 2)
	reachable, refuse = true, "Notification"
	if err := AppendSpool(home, "bob", "Notification", map[string]any{}, "r3", 3); err != nil {
		t.Fatal(err)
	}
	if n, err := h.Ingest(); err != nil || n != 3 {
		t.Fatalf("ingest: %d %v", n, err)
	}
	if len(got) != 2 || got[0] != (sent{"UserPromptSubmit", "r1"}) || got[1] != (sent{"Stop", "r2"}) {
		t.Fatalf("sent %v", got)
	}
	if n, _ := h.Ingest(); n != 0 {
		t.Fatalf("the spool was sent twice: %d", n)
	}
	if out := record("Stop", "r4", 4); out["nudge"] != "keep going" {
		t.Fatalf("the master's nudge did not come back: %v", out)
	}
	if tracker.State("bob") != turns.Idle {
		t.Fatal("the runner's tracker is not idle after the Stop")
	}
}
