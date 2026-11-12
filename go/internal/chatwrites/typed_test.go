package chatwrites

import (
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// typedRig is a rig whose delivery types at once (typesNow), so a person's message to a terminal agent is held until typed.
func typedRig(t *testing.T) *rig {
	was := typesNow
	typesNow = func(*supervisor.Env) bool { return true }
	t.Cleanup(func() { typesNow = was })
	return newRig(t)
}

// typeSoon marks the newest message to agent typed shortly after it was written, as the delivery would.
func (r *rig) typeSoon(agent string) {
	go func() {
		for i := 0; i < 200; i++ {
			var rid int64
			if r.db.QueryRow("SELECT id FROM requests WHERE target = ? ORDER BY id DESC LIMIT 1", agent).Scan(&rid) == nil {
				time.Sleep(50 * time.Millisecond)
				db.MarkDelivered(r.db, rid)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// The founder's message to a terminal agent answers once it is typed; past the limit an untyped one is taken back and the call is refused.
func TestAPersonsMessageToATerminalAgentAnswersOnceTypedAndIsTakenBackAtTheLimit(t *testing.T) {
	r := typedRig(t)
	srv := r.server("")
	r.typeSoon(r.k("alice"))
	if res := r.result(r.one(srv, "chat.send", `{"agent":"alice","body":"hi","request_id":"ty1"}`)); res["request"] == nil {
		t.Fatalf("answered with the request: %v", res)
	}
	var delivered bool
	r.db.QueryRow("SELECT delivered_at IS NOT NULL FROM requests WHERE target = ? ORDER BY id DESC LIMIT 1", r.k("alice")).Scan(&delivered)
	if !delivered {
		t.Fatal("answered before it was typed")
	}

	typeWait = 50 * time.Millisecond
	defer func() { typeWait = 0 }()
	m := r.one(srv, "chat.send", `{"agent":"bob","body":"anyone?","request_id":"ty2"}`)
	r.refusedWith(m, "state")
	if msg := m["error"].(map[string]any)["message"].(string); !strings.Contains(msg, "was not typed into "+addr.Ref(r.k("bob"))+"'s session in time, so it was taken back") {
		t.Fatalf("%q", msg)
	}
	var state string
	r.db.QueryRow("SELECT state FROM requests WHERE target = ? ORDER BY id DESC LIMIT 1", r.k("bob")).Scan(&state)
	if state != "withdrawn" {
		t.Fatalf("taken back, not left to be typed later: %s", state)
	}
	if again := r.one(srv, "chat.send", `{"agent":"bob","body":"anyone?","request_id":"ty2"}`); again["error"] == nil {
		t.Fatalf("a retry does not send it again: %v", again)
	}
}

// An agent's message (hl reply) is not held: it stays queued until the target takes it.
func TestAnAgentsMessageIsNotHeldUntilTyped(t *testing.T) {
	r := typedRig(t)
	typeWait = time.Hour
	defer func() { typeWait = 0 }()
	rid, err := db.AddRequest(r.db, db.NewRequest{Requester: r.k("alice"), Target: r.k("bob"), Body: "hello", Kind: "say"})
	if err != nil {
		t.Fatal(err)
	}
	w := &writes{e: r.env}
	start := time.Now()
	got, err := w.awaitTyped(&rpc.Conn{Caller: rpc.Caller{Role: "agent", Kind: "agent"}}, map[string]any{"request": rid}, nil)
	if err != nil || got.(map[string]any)["request"] != rid || time.Since(start) > time.Second {
		t.Fatalf("answered at once: %v %v", got, err)
	}
}
