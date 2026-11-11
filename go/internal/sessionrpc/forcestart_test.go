package sessionrpc

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// forceRig serves agent.force_start for ann, whose hold the fake lets past; unpassed records each take-back.
func forceRig(t *testing.T) (*rig, *conn, func() []string) {
	r := newRig(t)
	var mu sync.Mutex
	var unpassed []string
	r.e.ForceStart = func(agent string) (bool, string, error) {
		return true, "Starting " + agent + " now, even though the computer is busy.", nil
	}
	r.e.Unpass = func(agent string) { mu.Lock(); unpassed = append(unpassed, agent); mu.Unlock() }
	return r, r.dial(false), func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), unpassed...) }
}

// event writes an event for ann shortly after the call has let it past, as the tick's start would.
func (r *rig) eventSoon(kind, payload string) {
	go func() {
		time.Sleep(100 * time.Millisecond)
		r.e.DB.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(1, ?, ?, ?)", r.ann, kind, payload)
	}()
}

func TestForceStartAnswersOnceTheStartHasBegun(t *testing.T) {
	r, c, unpassed := forceRig(t)
	r.eventSoon("launching", "{}")
	got := result(t, c.call("agent.force_start", map[string]any{"agent": "ann", "request_id": rid("f1")}))
	if got["said"] != r.ann+" has started, even though the computer is busy." || len(unpassed()) != 0 {
		t.Fatalf("answered once started: %v %v", got, unpassed())
	}
}

func TestForceStartWhoseStartFailsIsRefusedWithWhy(t *testing.T) {
	r, c, _ := forceRig(t)
	r.eventSoon("wake_failed", `{"why": "no account free"}`)
	m := c.call("agent.force_start", map[string]any{"agent": "ann", "request_id": rid("f2")})
	if e, _ := m["error"].(map[string]any); e == nil || e["message"] != r.ann+": not started - no account free" {
		t.Fatalf("refused with why: %v", m)
	}
}

func TestForceStartWhoseStartDoesNotComeTakesThePassBack(t *testing.T) {
	r, c, unpassed := forceRig(t)
	r.e.Wait = 50 * time.Millisecond
	m := c.call("agent.force_start", map[string]any{"agent": "ann", "request_id": rid("f3")})
	e, _ := m["error"].(map[string]any)
	if msg, _ := e["message"].(string); !strings.Contains(msg, r.ann+" did not start in time, so what let it past is taken back") {
		t.Fatalf("refused at the limit: %v", m)
	}
	if got := unpassed(); len(got) != 1 || got[0] != r.ann {
		t.Fatalf("the pass is taken back: %v", got)
	}
}
