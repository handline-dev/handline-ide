package supervisor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// fakeTemps puts a task on a seat as go/internal/temps does (the lowest idle seat or a new one, marked at work, then the handoff) and gives
// it a session, or takes none when manual.
type fakeTemps struct {
	r      *rig
	manual bool
	noRoom bool // no further seat could start
	hired  []string
}

func (f *fakeTemps) Hire(agent string, tid int64, note string) string {
	if f.manual {
		return ""
	}
	seat := f.r.seat(agent, tid)
	f.r.agentUp(seat)
	f.r.must(taskwrites.HandOffAsDaemon(f.r.DB, f.r.home, f.r.Env, tid, seat, note))
	f.hired = append(f.hired, seat)
	return seat
}

// seat is agent's lowest idle seat, or a new one, at work on tid (0: idle).
func (r *rig) seat(agent string, tid int64) string {
	r.t.Helper()
	seats, err := team.TempsOf(r.DB, agent)
	r.must(err)
	seat := ""
	for _, s := range seats {
		if at, _ := team.HiredFor(r.DB, s); at == 0 {
			seat = s
			break
		}
	}
	if seat == "" {
		seat, err = team.NewSeat(r.DB, agent, r.clock)
		r.must(err)
	}
	r.must(team.SetHiredFor(r.DB, seat, tid))
	return seat
}

func (f *fakeTemps) Waiting(string) bool { return false }

// Placement is as go/internal/temps reads it: a seat of agent at work still holds an open task; Room unless noRoom.
func (f *fakeTemps) Placement(agent string) team.Placement {
	if f.manual {
		return team.Placement{}
	}
	p := team.Placement{Auto: true, Room: !f.noRoom}
	seats, err := team.TempsOf(f.r.DB, agent)
	f.r.must(err)
	for _, s := range seats {
		at, err := team.HiredFor(f.r.DB, s)
		f.r.must(err)
		held, err := team.Holds(f.r.DB, s)
		f.r.must(err)
		if at != 0 && held {
			p.Seated = true
		}
	}
	return p
}

// tempRig is alice mid-turn on T<first>, carol's, with T<second> from carol queued to her behind it.
func tempRig(t *testing.T, manual bool) (r *rig, temps *fakeTemps, first, second int64) {
	r = newRig(t)
	r.bigTeam()
	r.agentUp(r.k("alice"))
	temps = &fakeTemps{r: r, manual: manual}
	r.Temps = temps
	first = r.newTask("alice", "carol", "the screen")
	r.beginOnTop(first)
	r.must(db.SetAgentState(r.DB, r.k("alice"), states.Running, 0, ""))
	second = r.newTask("alice", "carol", "a typo on the settings page")
	return r, temps, first, second
}

func (r *rig) told(agent string, tid int64) bool {
	r.t.Helper()
	var n int
	r.must(r.DB.QueryRow(`SELECT COUNT(*) FROM requests q JOIN task_requests l ON l.request_id = q.id WHERE q.target = ? AND l.task_id = ?`,
		agent, tid).Scan(&n))
	return n > 0
}

// A task queued to an agent mid-turn on another goes to a seat of it, which starts on it, and the agent's turn gets nothing of it.
func TestATaskForAnAgentMidTurnGoesToASeatOfIt(t *testing.T) {
	r, temps, first, second := tempRig(t, false)
	typed := len(r.term.typed)
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 1 {
		t.Fatalf("hired %v", temps.hired)
	}
	temp := temps.hired[0]
	if got := r.acting(second); got != temp {
		t.Fatalf("T%d is %s's, want the temp %s's", second, got, temp)
	}
	if cur, ok, _ := db.CurrentTask(r.DB, temp); !ok || cur != second {
		t.Fatalf("the temp holds %d %v", cur, ok)
	}
	if !r.told(temp, second) {
		t.Fatal("the temp was not given the task")
	}
	if r.told(r.k("alice"), second) {
		t.Fatal("alice was told of the task her temp took")
	}
	for _, s := range r.term.typed[typed:] {
		if strings.Contains(s, "typo") && !strings.Contains(s, "temp of") {
			t.Fatalf("typed to alice: %q", s)
		}
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k("alice")); cur != first {
		t.Fatalf("alice holds %d, not her own T%d", cur, first)
	}
	if from, _ := team.Temps(r.DB); from[temp] != r.k("alice") {
		t.Fatalf("temps %v", from)
	}
}

// Manual, or no room: the task waits for the agent as it always did, and nobody else is given it.
func TestWithNoSeatTheTaskWaitsForTheBusyAgent(t *testing.T) {
	r, _, first, second := tempRig(t, true)
	r.must(r.SweepTaskQueue(r.clock))
	if got := r.acting(second); got != r.k("alice") {
		t.Fatalf("T%d went to %s", second, got)
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k("alice")); cur != first {
		t.Fatalf("alice holds %d", cur)
	}
	if r.told(r.k("alice"), second) {
		t.Fatal("alice was told of a task behind her current one")
	}
}

// An agent not mid-turn hires nobody: one that is free takes the task itself, as before.
func TestAnIdleAgentTakesTheTaskItself(t *testing.T) {
	r := newRig(t)
	r.bigTeam()
	r.agentUp(r.k("alice"))
	temps := &fakeTemps{r: r}
	r.Temps = temps
	tid := r.newTask("alice", "carol", "a typo")
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 0 {
		t.Fatalf("hired %v", temps.hired)
	}
	if cur, ok, _ := db.CurrentTask(r.DB, r.k("alice")); !ok || cur != tid {
		t.Fatalf("alice holds %d %v", cur, ok)
	}
}

// One sweep hires one seat: the next task waits until that seat's session is up, so each hire meets the limits on its own.
func TestOneSweepHiresOneSeat(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	third := r.newTask("alice", "carol", "another typo")
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 1 || r.acting(second) != temps.hired[0] || r.acting(third) != r.k("alice") {
		t.Fatalf("hired %v; T%d is %s's, T%d %s's", temps.hired, second, r.acting(second), third, r.acting(third))
	}
}

// requests is the bodies of the says to agent, oldest first.
func (r *rig) says(agent string) []string {
	r.t.Helper()
	rows, err := r.DB.Query("SELECT body FROM requests WHERE target = ? AND kind = 'say' ORDER BY id", agent)
	r.must(err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		r.must(rows.Scan(&b))
		out = append(out, b)
	}
	return out
}

// The agent passes the founder's word on as a comment on the task its seat holds: it reaches the seat mid-turn, as the founder's own
// comment there does.
func TestTheAgentsCommentOnItsSeatsTaskReachesTheSeatAsTheFoundersDoes(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	r.must(db.SetAgentState(r.DB, seat, states.Running, 0, ""))
	r.term.typed, r.term.modes = nil, nil
	r.CommentChanged(second, 0, r.k("alice"), "added", "use the new copy", false, nil)
	r.CommentChanged(second, 0, r.k("human"), "added", "and keep the old link", false, nil)
	got := r.says(seat)
	if len(got) != 2 || !strings.Contains(got[0], "use the new copy") || !strings.Contains(got[1], "keep the old link") {
		t.Fatalf("the seat was told %q", got)
	}
	if len(r.term.typed) != 2 || len(r.term.modes) != 2 || r.term.modes[0] != r.term.modes[1] {
		t.Fatalf("typed %q as %v: the agent's comment and the founder's go the same way", r.term.typed, r.term.modes)
	}
	if said := r.says(r.k("alice")); len(said) != 0 {
		t.Fatalf("alice was told her own comment: %q", said)
	}
}

// Once a seat's task is over and it sits idle, a comment on that task and a task handed to it reach the agent it is of.
func TestAnIdleSeatsWordsReachItsAgent(t *testing.T) {
	r, temps, _, second := tempRig(t, false)
	r.must(r.SweepTaskQueue(r.clock))
	seat := temps.hired[0]
	r.must(team.SetHiredFor(r.DB, seat, 0))
	r.CommentChanged(second, 0, r.k("carol"), "added", "thanks, one more thing", true, nil)
	if said := r.says(seat); len(said) != 0 {
		t.Fatalf("the idle seat was told %q", said)
	}
	if said := r.says(r.k("alice")); len(said) != 1 || !strings.Contains(said[0], "one more thing") {
		t.Fatalf("alice was told %q", said)
	}
	third := r.newTask("carol", "carol", "another typo")
	r.must(taskwrites.HandOffAsDaemon(r.DB, r.home, r.Env, third, seat, "yours"))
	if got := r.acting(third); got != r.k("alice") {
		t.Fatalf("handed to the idle seat, T%d is %s's", third, got)
	}
}

// A founder's message to an agent with a seat at work carries one line after the founder's words: task.request for new work, the tasks its
// seats hold, and where a word for one goes. An agent mid-turn with no seat at work gets only the first; a free one, none. The message
// still lands: the transcript ends with what was typed.
func TestTheFoundersMessageToABusyAgentSaysWhatRunsBesideIt(t *testing.T) {
	r, _, typed := typedRig(t)
	bob := r.k("bob")
	r.Temps = &fakeTemps{r: r}
	plain := r.founderSays(bob, "hi")
	if got, err := r.TellNow(r.DB, plain); err != nil || got != "sent" {
		t.Fatalf("%q %v", got, err)
	}
	eventually(t, func() bool { return len(typed()) == 1 })
	if typed()[0] != "hi" {
		t.Fatalf("a free agent with no seat at work: %q", typed()[0])
	}
	r.must(db.SetAgentState(r.DB, bob, states.Running, 0, ""))
	footer := r.newTask("bob", "carol", "the footer")
	r.seat(bob, footer)
	rid := r.founderSays(bob, "and the header?")
	if got, err := r.TellNow(r.DB, rid); err != nil || got != "sent" {
		t.Fatalf("%q %v", got, err)
	}
	eventually(t, func() bool { return len(typed()) == 2 })
	row, err := db.GetRequest(r.DB, rid)
	r.must(err)
	want := fmt.Sprintf("and the header?\n\nTo give this to a temp: `hl task request %s \"<title>\"`. "+
		"Your temps are on T%d \"the footer\"; a word for one goes there: `hl task comment T<n> add \"...\"`.", db.MsgNameOf(r.DB, row.Ref), footer)
	if typed()[1] != want {
		t.Fatalf("typed %q\nwant  %q", typed()[1], want)
	}
	eventually(t, func() bool { row, _ := db.GetRequest(r.DB, rid); return row.LandedAt.Valid })
}

// The founder's new work for a busy agent becomes a task with task.request, as that line says, and a seat of the agent takes it: it is
// the founder's, not one the agent queued for itself.
func TestTheFoundersNewWorkForABusyAgentGoesToASeatThroughTaskRequest(t *testing.T) {
	r, temps, first, _ := tempRig(t, false)
	r.must(r.SweepTaskQueue(r.clock)) // the queued T<second> takes the first seat
	rid := r.founderSays(r.k("alice"), "also fix the footer")
	r.must(db.MarkLanded(r.DB, rid)) // alice read it, mid-turn, and acts on it
	res, _ := r.callAs("alice", fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"task.request","params":{"request":%d,"request_id":"q1"}}`, rid))["result"].(map[string]any)
	if res == nil {
		t.Fatal("task.request refused")
	}
	tid := int64(res["task"].(float64))
	r.clock += HireRetryS
	r.must(r.SweepTaskQueue(r.clock))
	if len(temps.hired) != 2 || r.acting(tid) != temps.hired[1] {
		t.Fatalf("seats %v; T%d is %s's", temps.hired, tid, r.acting(tid))
	}
	if cur, _, _ := db.CurrentTask(r.DB, r.k("alice")); cur != first {
		t.Fatalf("alice holds %d, not her own T%d", cur, first)
	}
}
