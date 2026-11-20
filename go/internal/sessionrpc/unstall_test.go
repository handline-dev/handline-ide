package sessionrpc

import (
	"testing"
	"time"
)

// unstallRig is a stalled ann the daemon owes a session once cleared.
func unstallRig(t *testing.T) (*rig, *conn) {
	r := newRig(t)
	seedStall(t, r.e.DB, r.ann, "STALLED")
	r.e.StartOwed = func(string) bool { return true }
	return r, r.dial(false)
}

func TestUnstallAnswersOnceTheStartHasBegun(t *testing.T) {
	r, c := unstallRig(t)
	r.eventSoon("wake", "{}")
	got := result(t, c.call("agent.unstall", map[string]any{"agent": "ann", "request_id": rid("u1")}))
	if got["said"] != r.ann+": STALLED -> QUEUED; it has started" {
		t.Fatalf("answered once started: %v", got)
	}
}

// A start that fails, or one the tick holds back, is answered with the stall cleared and why: the clear is what the call did.
func TestUnstallWhoseStartFailsSaysWhy(t *testing.T) {
	r, c := unstallRig(t)
	r.eventSoon("wake_failed", `{"why": "no account free"}`)
	got := result(t, c.call("agent.unstall", map[string]any{"agent": "ann", "request_id": rid("u2")}))
	if got["said"] != r.ann+": STALLED -> QUEUED; not started - no account free" {
		t.Fatalf("failed: %v", got)
	}
	r, c = unstallRig(t)
	r.e.Now = func() time.Time { return time.Unix(500, 0) }
	r.e.StartFailed = func(_ string, since int64) string { // only a failure noted after the clear counts
		if since != 500 {
			return ""
		}
		return "could not be started - Waiting: new work is paused"
	}
	got = result(t, c.call("agent.unstall", map[string]any{"agent": "ann", "request_id": rid("u3")}))
	if got["said"] != r.ann+": STALLED -> QUEUED; not started - could not be started - Waiting: new work is paused" {
		t.Fatalf("held: %v", got)
	}
}

func TestUnstallWhoseStartDoesNotComeOrIsNotOwedSaysSo(t *testing.T) {
	r, c := unstallRig(t)
	r.e.Wait = 50 * time.Millisecond
	got := result(t, c.call("agent.unstall", map[string]any{"agent": "ann", "request_id": rid("u4")}))
	if got["said"] != r.ann+": STALLED -> QUEUED; its start has not begun yet" {
		t.Fatalf("at the limit: %v", got)
	}
	r, c = unstallRig(t)
	r.e.StartOwed = func(string) bool { return false }
	got = result(t, c.call("agent.unstall", map[string]any{"agent": "ann", "request_id": rid("u5")}))
	if got["said"] != r.ann+": STALLED -> QUEUED" {
		t.Fatalf("nothing owed: %v", got)
	}
}

// A launch the tick writes before it notes "could not be started" (its session file not up yet) is a start: the event wins.
func TestUnstallWhoseLaunchCameWithAFailureNoteHasStarted(t *testing.T) {
	r, c := unstallRig(t)
	r.eventSoon("launching", "{}")
	r.e.StartFailed = func(string, int64) string {
		var n int
		r.e.DB.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'launching'").Scan(&n)
		if n == 0 {
			return ""
		}
		return "could not be started - its session is not up yet"
	}
	got := result(t, c.call("agent.unstall", map[string]any{"agent": "ann", "request_id": rid("u6")}))
	if got["said"] != r.ann+": STALLED -> QUEUED; it has started" {
		t.Fatalf("a launch with a note: %v", got)
	}
}
