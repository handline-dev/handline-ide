package lifecycle

import "testing"

func TestCloseVerdictTable(t *testing.T) {
	base := closeInput{SessionID: "s", Now: 10000, IdleCloseS: 600, Timeout: 300, IdleKnown: true, IdleSince: 9000}
	with := func(f func(*closeInput)) closeInput { in := base; f(&in); return in }
	for _, c := range []struct {
		name string
		in   closeInput
		want bool
	}{
		{"idle past its window", base, true},
		{"inside its window", with(func(i *closeInput) { i.IdleSince = 9900 }), false},
		{"inside its window but it answered the note ask", with(func(i *closeInput) { i.IdleSince = 9900; i.AnsweredTurn = true }), true},
		{"inside its window, background work done and may stop", with(func(i *closeInput) { i.IdleSince = 9900; i.BgOwesNothing, i.BgMayStop = true, true }), true},
		{"inside its window, background work owes something", with(func(i *closeInput) { i.IdleSince = 9900; i.BgMayStop = true }), false},
		{"working", with(func(i *closeInput) { i.Busy = true }), false},
		{"no session", with(func(i *closeInput) { i.NoSession = true }), false},
		{"background work holds it", with(func(i *closeInput) { i.BgHolds = true }), false},
		{"no session id to resume", with(func(i *closeInput) { i.SessionID = "" }), false},
		{"work waiting and the cache warm", with(func(i *closeInput) { i.Queued = true }), false},
		{"work waiting and the cache gone", with(func(i *closeInput) { i.Queued, i.CacheExpired = true, true }), true},
		{"work waiting but parked", with(func(i *closeInput) { i.Queued, i.Parked = true, true }), true},
		{"its window is never", with(func(i *closeInput) { i.TimeoutNever = true }), false},
		{"a close failed within the idle window", with(func(i *closeInput) { i.CloseFailed, i.CloseFailedAt = true, 9700 }), false},
		{"a close failed long ago", with(func(i *closeInput) { i.CloseFailed, i.CloseFailedAt = true, 9000 }), true},
		{"nothing ever seen from it", with(func(i *closeInput) { i.IdleKnown = false }), false},
	} {
		if got := closeVerdict(c.in); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}
