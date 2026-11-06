package verdict

import "testing"

func TestTaskState(t *testing.T) {
	for _, c := range []struct {
		f    TaskFacts
		want string
	}{
		{TaskFacts{}, "draft"},
		{TaskFacts{Held: true}, "draft"},
		{TaskFacts{Queued: true}, "queued"},
		{TaskFacts{Sent: true}, "queued"},                             // asked, its owner holds another
		{TaskFacts{Begun: true, Held: true, Blocked: true}, "queued"}, // held, but blocked
		{TaskFacts{Sent: true, Held: true}, "doing"},                  // asked, held, unblocked
		{TaskFacts{Begun: true, Held: true, Queued: true}, "doing"},   // begun counts as asked
		{TaskFacts{Done: true, Sent: true, Held: true}, "done"},       // written states win
		{TaskFacts{Discarded: true, Done: true}, "discarded"},         // a discard over a done
		{TaskFacts{Queued: true, Blocked: true}, "queued"},
	} {
		if got := TaskState(c.f); got != c.want {
			t.Errorf("%+v: %s, want %s", c.f, got, c.want)
		}
	}
}
