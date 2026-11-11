package tick

import (
	"database/sql"
	"fmt"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/taskboard/msgtest"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

// tbPorts scripts the timebox's foreign calls and keeps the notices it sent.
type tbPorts struct {
	TimeboxPorts
	doing      map[int64]bool
	boss       string
	bosses     map[string]string // an agent's own lead, over boss
	lead       string
	sendOK     bool
	sent       []string // "who: text", typed at once
	batched    []string // "who: text", for the digest
	said       []string // "who: text", a say that wakes a closed session
	handed     []string // "T<n> to who: note"
	handErr    error    // what HandOffHeld answers
	handTries  int
	bgStale    bool
	bgOwes     bool
	never      bool
	silent     int64
	haveSilent bool
	since      int64
	haveSince  bool
	prompted   int64
	noticeS    int64
	graceS     int64
	checkS     int64
	jobs       []string
	forgotten  *Forgotten
	alarmOn    bool // blocker_alarm, with 100 s for the holder and 400 s for the lead
}

func (p *tbPorts) HandOffHeld(_ db.Q, tid int64, to, note string) error {
	p.handTries++
	if p.handErr != nil {
		return p.handErr
	}
	p.handed = append(p.handed, fmt.Sprintf("T%d to %s: %s", tid, to, note))
	return nil
}
func (p *tbPorts) TaskDoing(_ db.Q, t *db.Task) bool { return p.doing[t.ID] }
func (p *tbPorts) SupervisorOf(agent string) string {
	if s, ok := p.bosses[agent]; ok {
		return s
	}
	return p.boss
}
func (p *tbPorts) BgStale(db.Q, string, int64) bool            { return p.bgStale }
func (p *tbPorts) BgOwesNothing(db.Q, string) bool             { return p.bgOwes }
func (p *tbPorts) NeverCloses(string) bool                     { return p.never }
func (p *tbPorts) BgSilentS(db.Q, string, int64) (int64, bool) { return p.silent, p.haveSilent }
func (p *tbPorts) BgSince(db.Q, string) (int64, bool)          { return p.since, p.haveSince }
func (p *tbPorts) BgPromptedAt(db.Q, string) int64             { return p.prompted }
func (p *tbPorts) StaleBgVetoS() int64                         { return 600 }
func (p *tbPorts) BgNoticeS(string) int64                      { return p.noticeS }
func (p *tbPorts) BgGraceS(string) int64                       { return p.graceS }
func (p *tbPorts) BgCheckS(string) int64                       { return p.checkS }
func (p *tbPorts) BgJobs(db.Q, string, int, int64) []string    { return p.jobs }
func (p *tbPorts) NudgeScheduleLen() int                       { return 3 }
func (p *tbPorts) BlockerAlarm() (bool, int64, int64)          { return p.alarmOn, 100, 400 }
func (p *tbPorts) ForgottenResolve(db.Q, string, *db.AgentState, int64) *Forgotten {
	return p.forgotten
}
func (p *tbPorts) SendBatch(_ db.Q, who, text string) error {
	p.batched = append(p.batched, who+": "+text)
	return nil
}
func (p *tbPorts) SayNow(_ db.Q, who, text string) error {
	p.said = append(p.said, who+": "+text)
	return nil
}
func (p *tbPorts) SendNotice(_ db.Q, who, text string) bool {
	p.sent = append(p.sent, who+": "+text)
	return p.sendOK
}

func timebox(t *testing.T) (*Tick, *world, *Timebox, *tbPorts) {
	tk, w := newWorld(t)
	p := &tbPorts{doing: map[int64]bool{}, sendOK: true}
	return tk, w, &Timebox{Ports: p}, p
}

func newTask(t *testing.T, tk *Tick, owner, opener string, timeoutS int64) int64 {
	t.Helper()
	tid, err := db.NewTask(tk.Q, "the work  on the thing", owner, opener, 0, db.DefaultPriority, "report", timeoutS, "")
	if err != nil {
		t.Fatal(err)
	}
	return tid
}

func TestATaskThatIsDoingStartsItsClockThenTellsItsHolderAndLeadNotItsOpenerPastTheDeadlineAndEachLengthOn(t *testing.T) {
	tk, w, b, p := timebox(t)
	p.boss = "lead"
	tid := newTask(t, tk, "a", "boss", 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	task, _ := db.GetTask(tk.Q, tid)
	if !task.TimeoutAt.Valid || task.TimeoutAt.Int64 != 1100 || len(p.batched) != 0 {
		t.Fatalf("the clock starts where it becomes doing: %+v %v", task, p.batched)
	}
	b.SweepTaskTimeouts(tk, 1099)
	if len(p.batched) != 0 {
		t.Fatalf("told before the deadline: %v", p.batched)
	}
	b.SweepTaskTimeouts(tk, 1200)
	if len(p.batched) != 2 || !strings.HasPrefix(p.batched[0], "a: T") || !strings.HasPrefix(p.batched[1], "lead: T") || !strings.Contains(p.batched[0], "the work on the thing") ||
		!strings.Contains(p.batched[0], "held by a") || !strings.Contains(p.batched[0], "mark what blocks it") || strings.Contains(p.batched[0], "--timeout") {
		t.Fatalf("%v", p.batched)
	}
	if !strings.Contains(p.batched[0], `hl task release T1 "<reason>"`) {
		t.Fatalf("any doing task can be released, with a reason: %v", p.batched)
	}
	if len(p.sent) != 0 {
		t.Fatalf("the notices wait in the digest: %v", p.sent)
	}
	task, _ = db.GetTask(tk.Q, tid)
	if task.TimeoutAt.Int64 != 1100 {
		t.Fatalf("an overrun clock runs on from its end: %+v", task)
	}
	b.SweepTaskTimeouts(tk, 1299)
	if len(p.batched) != 2 {
		t.Fatalf("told again before a further length: %v", p.batched)
	}
	b.SweepTaskTimeouts(tk, 1300)
	if len(p.batched) != 4 {
		t.Fatalf("not told again a length after the last notice: %v", p.batched)
	}
	p.batched = p.batched[:2]
	if n := countEvents(t, tk, "a", "task_timeout_exceeded"); n != 2 || !w.logged("past its") {
		t.Fatalf("the event is on the holder: events %d, log %v", n, w.logs)
	}
	if n := countEvents(t, tk, "boss", "task_timeout_exceeded"); n != 0 {
		t.Fatalf("an opener that does not hold the task has %d timeout events", n)
	}
	var told string
	if err := tk.Q.QueryRow("SELECT json_extract(payload, '$.notified') FROM events WHERE kind = 'task_timeout_exceeded' LIMIT 1").Scan(&told); err != nil || told != `["a","lead"]` {
		t.Fatalf("told %q %v", told, err)
	}
}

func TestEveryOwnersTaskIsClocked(t *testing.T) {
	tk, _, b, p := timebox(t)
	mine := newTask(t, tk, "a", "boss", 100)
	terminal := newTask(t, tk, "term", "boss", 100)
	p.doing[mine], p.doing[terminal] = true, true
	b.SweepTaskTimeouts(tk, 1000)
	b.SweepTaskTimeouts(tk, 1200)
	if task, _ := db.GetTask(tk.Q, terminal); !task.TimeoutAt.Valid || task.TimeoutAt.Int64 != 1100 || countEvents(t, tk, "term", "task_timeout_exceeded") != 1 {
		t.Fatalf("a terminal agent's task is clocked too: %+v", task)
	}
}

func countEvents(t *testing.T, tk *Tick, agent, kind string) int {
	t.Helper()
	var n int
	if err := tk.Q.QueryRow("SELECT COUNT(*) FROM events WHERE agent = ? AND kind = ?", agent, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestATaskThatStopsBeingDoneHasItsClockPausedAndItsOverrunNoticeWaitsForTheDigest(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	p.doing[tid] = false
	b.SweepTaskTimeouts(tk, 1010)
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Valid {
		t.Fatalf("a task that is not doing keeps a deadline: %+v", task)
	}
	// Past the deadline with the holder's session busy: the notice waits in its digest, so nothing is retried.
	p.doing[tid], p.sendOK = true, false
	b.SweepTaskTimeouts(tk, 2000)
	b.SweepTaskTimeouts(tk, 2200)
	b.SweepTaskTimeouts(tk, 2200+NoticeRetryS+1)
	if len(p.batched) != 1 || len(p.sent) != 0 || countEvents(t, tk, "a", "task_timeout_exceeded") != 1 {
		t.Fatalf("%v %v", p.batched, p.sent)
	}
}

func TestAWaitingTaskStopsItsClockAndGoesOnFromWhatItHadLeftWhenTheWaitEnds(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 100)
	blocker := newTask(t, tk, "c", "boss", 0)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	if err := db.AddBlock(tk.Q, tid, blocker); err != nil {
		t.Fatal(err)
	}
	b.SweepTaskTimeouts(tk, 1040)
	b.SweepTaskTimeouts(tk, 1500) // past the first deadline, 1100
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Valid || len(p.batched) != 0 {
		t.Fatalf("a blocked task's clock ran: %+v %v", task, p.batched)
	}
	if _, err := db.RemoveBlock(tk.Q, tid, blocker); err != nil {
		t.Fatal(err)
	}
	b.SweepTaskTimeouts(tk, 1500)
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Int64 != 1560 {
		t.Fatalf("the clock did not go on from the 60s it had left: %+v", task)
	}
	b.SweepTaskTimeouts(tk, 1559)
	b.SweepTaskTimeouts(tk, 1560)
	if len(p.batched) != 1 || !strings.HasPrefix(p.batched[0], "a: T") {
		t.Fatalf("the holder is told when what was left has run: %v", p.batched)
	}

	// over, then an open child of its own: the clock stands still, over by as much, while the child is open
	child, err := db.NewTask(tk.Q, "part", "c", "a", tid, db.DefaultPriority, "report", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	b.SweepTaskTimeouts(tk, 1570)
	b.SweepTaskTimeouts(tk, 9000)
	if _, err := tk.Q.Exec("UPDATE tasks SET done_evidence = 'x' WHERE id = ?", child); err != nil {
		t.Fatal(err)
	}
	b.SweepTaskTimeouts(tk, 9000)
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Int64 != 8990 || len(p.batched) != 1 {
		t.Fatalf("a task waiting on its child goes on over by the 10s it was: %+v %v", task, p.batched)
	}
}

func TestAWaitAcrossADaemonRestartNeitherLosesNorRepeatsAReminder(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 100)
	blocker := newTask(t, tk, "c", "boss", 0)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	if err := db.AddBlock(tk.Q, tid, blocker); err != nil {
		t.Fatal(err)
	}
	b.SweepTaskTimeouts(tk, 1050)
	// a new daemon: nothing carried but the db
	b2 := &Timebox{Ports: p}
	b2.SweepTaskTimeouts(tk, 5000)
	if len(p.batched) != 0 {
		t.Fatalf("reminded while waiting: %v", p.batched)
	}
	if _, err := tk.Q.Exec("UPDATE tasks SET done_evidence = 'x' WHERE id = ?", blocker); err != nil {
		t.Fatal(err)
	}
	b2.SweepTaskTimeouts(tk, 5000)
	b3 := &Timebox{Ports: p}
	b3.SweepTaskTimeouts(tk, 5049)
	if len(p.batched) != 0 {
		t.Fatalf("told before the 50s it had left ran: %v", p.batched)
	}
	b3.SweepTaskTimeouts(tk, 5050)
	b3.SweepTaskTimeouts(tk, 5050+NoticeRetryS)
	if len(p.batched) != 1 {
		t.Fatalf("told once, when what it had left ran: %v", p.batched)
	}
}

func TestATaskOffDoingResumesWithWhatItHadLeft(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	p.doing[tid] = false
	b.SweepTaskTimeouts(tk, 1040)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 2000)
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Int64 != 2060 {
		t.Fatalf("the clock goes on from what it had left: %+v", task)
	}
}

func TestATaskOpenedByTheFounderTellsOnlyItsHolderAndLogsAnEvent(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", actor.OwnerKey, 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	b.SweepTaskTimeouts(tk, 1200)
	if len(p.batched) != 1 || countEvents(t, tk, "a", "task_timeout_exceeded") != 1 || countEvents(t, tk, actor.OwnerKey, "task_timeout_exceeded") != 0 {
		t.Fatalf("only the holder is told, and the event is on it: %v", p.batched)
	}
}

func TestStaleBackgroundWorkTellsTheSupervisorOnceAndClearsWhenItIsNoLongerStale(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.boss, p.bgStale, p.silent, p.since, p.haveSince = "lead", true, 700, 100, true
	b.SweepStaleBackground(tk, 1000)
	b.SweepStaleBackground(tk, 1100)
	if len(p.sent) != 1 || !strings.Contains(p.sent[0], "background work for") || countEvents(t, tk, "a", "stale_background") != 1 {
		t.Fatalf("%v", p.sent)
	}
	p.bgStale = false
	b.SweepStaleBackground(tk, 1200)
	p.bgStale = true
	b.SweepStaleBackground(tk, 1300+NoticeRetryS)
	if len(p.sent) != 2 {
		t.Fatalf("a later stretch is told again: %v", p.sent)
	}
	// An agent set to never close is not told.
	tk, _, b, p = timebox(t)
	p.bgStale, p.never = true, true
	b.SweepStaleBackground(tk, 1000)
	if len(p.sent) != 0 {
		t.Fatalf("%v", p.sent)
	}
}

func TestForgottenBackgroundWorkIsToldToFinishOnceItsIdleWindowIsOver(t *testing.T) {
	tk, w, b, p := timebox(t)
	w.bgWork = true
	p.bgOwes, p.noticeS, p.graceS, p.haveSilent, p.silent = true, 600, 300, true, 599
	b.SweepForgottenBackground(tk, 1000)
	if len(p.sent) != 0 {
		t.Fatalf("%v", p.sent)
	}
	p.silent = 700
	b.SweepForgottenBackground(tk, 1100)
	if len(p.sent) != 1 || !strings.HasPrefix(p.sent[0], "a: You have shown nothing") || countEvents(t, tk, "a", "background_prompted") != 1 {
		t.Fatalf("%v", p.sent)
	}
	// Work that is owed (a request open) is not forgotten work.
	tk, w, b, p = timebox(t)
	w.bgWork, p.bgOwes, p.haveSilent, p.silent, p.noticeS = true, false, true, 9999, 600
	b.SweepForgottenBackground(tk, 1000)
	if len(p.sent) != 0 {
		t.Fatalf("%v", p.sent)
	}
}

func TestTheBackgroundCheckAsksAnIdleAgentWhichJobsToStopEveryCheckInterval(t *testing.T) {
	tk, w, b, p := timebox(t)
	w.bgWork = true
	setState(t, tk, "a", states.Idle, 0)
	p.since, p.haveSince, p.checkS, p.jobs = 100, true, 600, []string{"npm test"}
	b.SweepBackgroundCheck(tk, 500)
	if len(p.sent) != 0 {
		t.Fatalf("asked before the interval: %v", p.sent)
	}
	b.SweepBackgroundCheck(tk, 800)
	if len(p.sent) != 1 || !strings.Contains(p.sent[0], "1. npm test") || !strings.Contains(p.sent[0], "a process you detached (nohup, &, PPID 1) is not asked about") || countEvents(t, tk, "a", "background_checked") != 1 {
		t.Fatalf("%v", p.sent)
	}
	b.SweepBackgroundCheck(tk, 900)
	if len(p.sent) != 1 {
		t.Fatalf("asked again inside the interval: %v", p.sent)
	}
	// A notice that could not be sent leaves no check behind.
	tk, w, b, p = timebox(t)
	w.bgWork, p.sendOK = true, false
	setState(t, tk, "a", states.Idle, 0)
	p.since, p.haveSince, p.checkS = 100, true, 600
	b.SweepBackgroundCheck(tk, 800)
	if countEvents(t, tk, "a", "background_checked") != 0 {
		t.Fatal("an ask that was not sent is recorded as asked")
	}
}

func TestAHeadNobodyResolvedTellsItsTasksOpenerOnceItsNudgeScheduleIsUsedUp(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.boss = "lead"
	tid := newTask(t, tk, "a", "asker", 0)
	if _, err := tk.Q.Exec("INSERT INTO requests(id, ts, requester, target, body, state, edge_ok) VALUES(7, 1, 'asker', 'a', 'x', 'landed', 1), " +
		"(8, 1, 'lead', 'a', 'y', 'landed', 1)"); err != nil {
		t.Fatal(err)
	}
	p.forgotten = &Forgotten{HeadID: 7, IdleS: 3600, Behind: 2, Exhausted: true, Task: &db.Task{ID: tid}}
	b.SweepForgottenResolveCapped(tk, 1000)
	b.SweepForgottenResolveCapped(tk, 1100)
	if len(p.sent) != 1 || !strings.HasPrefix(p.sent[0], "asker: ") || !strings.Contains(msgtest.IDs(tk.Q, p.sent[0]), fmt.Sprintf("T%d (#7)", tid)) || !strings.Contains(p.sent[0], "after 3 reminders") || !strings.Contains(p.sent[0], "2 more waiting") {
		t.Fatalf("%v", p.sent)
	}
	// Blocked on record, or not yet exhausted: nothing, and the mark clears so a later head is told.
	p.forgotten = &Forgotten{HeadID: 7, Exhausted: false}
	b.SweepForgottenResolveCapped(tk, 1200)
	// A head that is no task goes to the supervisor.
	p.forgotten = &Forgotten{HeadID: 8, IdleS: 100, Exhausted: true}
	b.SweepForgottenResolveCapped(tk, 1300+NoticeRetryS)
	if len(p.sent) != 2 || !strings.HasPrefix(p.sent[1], "lead: ") || !strings.Contains(msgtest.IDs(tk.Q, p.sent[1]), "#8") {
		t.Fatalf("%v", p.sent)
	}
}

func TestOneNoticesFailureStopsTheRestAsAnExceptionDid(t *testing.T) {
	tk, _, b, p := timebox(t)
	setState(t, tk, "a", states.Idle, 0)
	p.forgotten = &Forgotten{HeadID: 1}
	// BgStale panics through the guard: Sweep returns that as its error and the later notice does not run.
	b.Ports = &panicking{p}
	if err := b.Sweep(tk, 1000); err == nil {
		t.Fatal("a failed notice was swallowed")
	}
}

type panicking struct{ *tbPorts }

func (p *panicking) BgStale(db.Q, string, int64) bool { panic("boom") }

func TestATaskGivenNoTimeoutRunsFourHoursBeforeItsOpenerIsTold(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 0)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Int64 != 1000+4*3600 {
		t.Fatalf("the default is 4h: %+v", task)
	}
	b.SweepTaskTimeouts(tk, 1000+4*3600-60)
	if len(p.batched) != 0 {
		t.Fatalf("told at 3h59m: %v", p.batched)
	}
	b.SweepTaskTimeouts(tk, 1000+4*3600+60)
	if len(p.batched) != 1 {
		t.Fatalf("not told at 4h01m: %v", p.batched)
	}
}

func TestATaskTheFounderHoldsRemindsNoAgentAndItsClockStartsFreshWhenItLeavesThem(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, actor.OwnerKey, "boss", 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	(&Timebox{Ports: p}).SweepTaskTimeouts(tk, 9000) // a restart in between
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Valid || len(p.batched) != 0 || countEvents(t, tk, actor.OwnerKey, "task_timeout_exceeded")+countEvents(t, tk, "boss", "task_timeout_exceeded") != 0 {
		t.Fatalf("the founder's task reminded its opener: %+v %v", task, p.batched)
	}
	if _, err := tk.Q.Exec("UPDATE tasks SET owner = 'a' WHERE id = ?", tid); err != nil {
		t.Fatal(err)
	}
	b.SweepTaskTimeouts(tk, 9000)
	b.SweepTaskTimeouts(tk, 9100)
	if len(p.batched) != 1 || !strings.HasPrefix(p.batched[0], "a: T") {
		t.Fatalf("back with an agent, a full stretch then one notice: %v", p.batched)
	}
}

func TestATaskWaitingOnTheFounderThroughAChainOrACycleIsNotReminded(t *testing.T) {
	tk, _, b, p := timebox(t)
	top := newTask(t, tk, "a", "boss", 100)
	mid := newTask(t, tk, "c", "boss", 100)
	founders, err := db.NewTask(tk.Q, "decide", actor.OwnerKey, "c", mid, db.DefaultPriority, "report", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddBlock(tk.Q, top, mid); err != nil {
		t.Fatal(err)
	}
	p.doing[top], p.doing[mid], p.doing[founders] = true, true, true
	b.SweepTaskTimeouts(tk, 1000)
	b.SweepTaskTimeouts(tk, 9000)
	if len(p.batched) != 0 {
		t.Fatalf("a chain ending at the founder reminded: %v", p.batched)
	}
	// blockers that wait on each other: each sweep still ends, and nothing is sent
	if err := db.AddBlock(tk.Q, mid, top); err != nil {
		t.Fatal(err)
	}
	b.SweepTaskTimeouts(tk, 20000)
	if len(p.batched) != 0 {
		t.Fatalf("a cycle reminded: %v", p.batched)
	}
}

// The stall this guards against: an agent whose session closed, by any path, while it held a doing task nothing waited on, and no delivery ever woke it.
func TestAClosedAgentHoldingADoingTaskIsAskedOnceToActOnIt(t *testing.T) {
	// held sets up agent a holding a doing task, closed with no idle_close event, and answers what a sweep (through Sweep, as the daemon runs it) says
	// once the case has changed it.
	held := func(t *testing.T, change func(tk *Tick, b *Timebox, p *tbPorts, tid int64)) []string {
		tk, _, b, p := timebox(t)
		tid := newTask(t, tk, "a", "boss", 100)
		p.doing[tid] = true
		check(db.SetCurrentTask(tk.Q, "a", tid, true))
		check(db.SetAgentState(tk.Q, "a", states.Idle, 0, ""))
		if _, err := tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", db.Now()-10); err != nil {
			t.Fatal(err)
		}
		if change != nil {
			change(tk, b, p, tid)
		}
		check(b.Sweep(tk, 1000))
		check(b.Sweep(tk, 1100))
		return p.said
	}
	if said := held(t, nil); len(said) != 1 || !strings.HasPrefix(said[0], "a: You hold T1") || !strings.Contains(said[0], "hand it on") || !strings.Contains(said[0], "--blocked-by") {
		t.Fatalf("asked once, with the ways out: %v", said)
	}
	for name, change := range map[string]func(tk *Tick, b *Timebox, p *tbPorts, tid int64){
		"its session is open": func(tk *Tick, _ *Timebox, _ *tbPorts, _ int64) {
			tk.Q.Exec("UPDATE agent_state SET closed_at = NULL WHERE agent = 'a'")
		},
		"the task is not doing": func(_ *Tick, _ *Timebox, p *tbPorts, tid int64) { p.doing[tid] = false },
		"the task waits on another": func(tk *Tick, _ *Timebox, _ *tbPorts, tid int64) {
			check(db.AddBlock(tk.Q, tid, newTask(t, tk, "c", "boss", 100)))
		},
		"the task waits on an open child": func(tk *Tick, _ *Timebox, _ *tbPorts, tid int64) {
			if _, err := db.NewTask(tk.Q, "kid", "c", "a", tid, db.DefaultPriority, "report", 100, ""); err != nil {
				t.Fatal(err)
			}
		},
		"the current task is another agent's": func(tk *Tick, _ *Timebox, p *tbPorts, tid int64) {
			other := newTask(t, tk, "c", "boss", 100)
			p.doing[other] = true
			check(db.SetCurrentTask(tk.Q, "a", other, true))
		},
	} {
		if said := held(t, change); len(said) != 0 {
			t.Errorf("asked although %s: %v", name, said)
		}
	}
}

// A holder that, closed again after the ask, left the task as it was passes it to whom it answers to, with the queue behind it named.
func TestATaskStillHeldAfterTheAskIsHandedOn(t *testing.T) {
	run := func(t *testing.T, change func(tk *Tick, tid int64)) *tbPorts {
		tk, _, b, p := timebox(t)
		tid := newTask(t, tk, "a", "boss", 100)
		newTask(t, tk, "a", "boss", 100)
		p.doing[tid] = true
		check(db.SetCurrentTask(tk.Q, "a", tid, true))
		check(db.SetAgentState(tk.Q, "a", states.Idle, 0, ""))
		tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", int64(500))
		check(b.Sweep(tk, 1000)) // asked
		if len(p.said) != 1 {
			t.Fatalf("asked once: %v", p.said)
		}
		tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", int64(1500)) // closed again
		if change != nil {
			change(tk, tid)
		}
		check(b.Sweep(tk, 2000))
		return p
	}
	if p := run(t, nil); len(p.handed) != 1 || !strings.HasPrefix(p.handed[0], "T1 to boss: held after being asked; queue behind it: T2") {
		t.Fatalf("handed to who it answers to, naming the queue: %v", p.handed)
	}
	if p := run(t, func(tk *Tick, _ int64) {
		tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", int64(900))
	}); len(p.handed) != 0 {
		t.Fatalf("handed although it did not close again after the ask: %v", p.handed)
	}
	if p := run(t, func(tk *Tick, tid int64) { must(db.AddMemo(tk.Q, tid, "a", "working on it")) }); len(p.handed) != 0 {
		t.Fatalf("handed although it wrote a memo since the ask: %v", p.handed)
	}
}

// heldAfterAsk sets up agent a closed holding a doing task T1 (opened by opener) that was asked at 1000 and closed again at 1500.
func heldAfterAsk(t *testing.T, opener string, boss string) (*Tick, *Timebox, *tbPorts, int64) {
	tk, _, b, p := timebox(t)
	p.boss = boss
	tid := newTask(t, tk, "a", opener, 100)
	p.doing[tid] = true
	check(db.SetCurrentTask(tk.Q, "a", tid, true))
	check(db.SetAgentState(tk.Q, "a", states.Idle, 0, ""))
	tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", int64(500))
	check(b.Sweep(tk, 1000))
	tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", int64(1500))
	return tk, b, p, tid
}

func TestAHolderThatWorkedAfterTheAskIsNotHandedOnAndIsAskedAnewAtItsNextClose(t *testing.T) {
	// the ask's own turn is one prompt and one stop: it does not count as work
	tk, b, p, tid := heldAfterAsk(t, "boss", "lead")
	db.AddEventAt(tk.Q, 1010, "a", "UserPromptSubmit")
	db.AddEventAt(tk.Q, 1020, "a", "Stop")
	check(b.Sweep(tk, 2000))
	if len(p.handed) != 1 {
		t.Fatalf("the ask's own turn is not work: %v", p.handed)
	}
	for name, add := range map[string]func(tk *Tick){
		"a commit":      func(tk *Tick) { db.AddEventAt(tk.Q, 1100, "a", "commit") },
		"a second stop": func(tk *Tick) { db.AddEventAt(tk.Q, 1030, "a", "Stop"); db.AddEventAt(tk.Q, 1040, "a", "Stop") },
		"a memo at the second of the ask": func(tk *Tick) {
			must(db.AddMemo(tk.Q, tid, "a", "x"))
			tk.Q.Exec("UPDATE task_memos SET created_at = 1000")
		},
	} {
		tk, b, p, _ := heldAfterAsk(t, "boss", "lead")
		add(tk)
		check(b.Sweep(tk, 2000))
		if len(p.handed) != 0 || len(p.said) != 2 {
			t.Errorf("%s: worked, so asked anew and not handed on: handed %v said %v", name, p.handed, p.said)
		}
		tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", int64(2500))
		check(b.Sweep(tk, 3000))
		if len(p.handed) != 1 {
			t.Errorf("%s: closed again without working after the new ask, handed on: %v", name, p.handed)
		}
	}
}

func TestAHeldTaskGoesToTheBossWhenItAnswersToTheHolderAndIsNeverHandedToTheFounder(t *testing.T) {
	tk, b, p, _ := heldAfterAsk(t, "a", "lead")
	check(b.Sweep(tk, 2000))
	if len(p.handed) != 1 || !strings.HasPrefix(p.handed[0], "T1 to lead:") {
		t.Fatalf("a task answering to its holder goes to the boss: %v", p.handed)
	}
	for _, opener := range []string{actor.OwnerKey, "a"} {
		tk, b, p, _ := heldAfterAsk(t, opener, "")
		check(b.Sweep(tk, 2000))
		if len(p.handed) != 0 || p.handTries != 0 {
			t.Errorf("opened by %s with no boss: not handed to the founder: %v", opener, p.handed)
		}
	}
}

func TestARefusedHandoffIsNotTriedAgainEveryTick(t *testing.T) {
	tk, b, p, _ := heldAfterAsk(t, "boss", "lead")
	p.handErr = fmt.Errorf("refused")
	check(b.Sweep(tk, 2000))
	check(b.Sweep(tk, 2030))
	if p.handTries != 1 {
		t.Fatalf("one try inside the backoff: %d", p.handTries)
	}
	p.handErr = nil
	check(b.Sweep(tk, 2000+HandOffRetryS))
	if len(p.handed) != 1 {
		t.Fatalf("tried again after the backoff: %v", p.handed)
	}
}

// queuedBlocker is a queued task of a's, opened by boss, with task waiter blocked on it since 1000.
func queuedBlocker(t *testing.T, tk *Tick) (blocker, waiter int64) {
	t.Helper()
	blocker, waiter = newTask(t, tk, "a", "boss", 0), newTask(t, tk, "b", "boss", 0)
	if _, err := db.SetTaskQueued(tk.Q, blocker, "boss"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddBlock(tk.Q, waiter, blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := tk.Q.Exec("UPDATE task_blocks SET created_at = 1000"); err != nil {
		t.Fatal(err)
	}
	return blocker, waiter
}

func TestAQueuedTaskOthersWaitOnTellsItsHolderThenItsLeadOnceEach(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.boss = "lead"
	blocker, waiter := queuedBlocker(t, tk)
	b.SweepQueuedBlockers(tk, 2000)
	if len(p.batched) != 0 {
		t.Fatalf("told with blocker_alarm off: %v", p.batched)
	}
	p.alarmOn = true
	b.SweepQueuedBlockers(tk, 1099)
	if len(p.batched) != 0 {
		t.Fatalf("told before blocker_alarm_s: %v", p.batched)
	}
	b.SweepQueuedBlockers(tk, 1100)
	want := fmt.Sprintf("a: T%d (the work on the thing), held by a, is queued, and T%d has waited on it for", blocker, waiter)
	if len(p.batched) != 1 || !strings.HasPrefix(p.batched[0], want) || !strings.Contains(p.batched[0], fmt.Sprintf("hl task done T%d", blocker)) {
		t.Fatalf("%v", p.batched)
	}
	b.SweepQueuedBlockers(tk, 1399)
	if len(p.batched) != 1 {
		t.Fatalf("the holder is told once, the lead not before blocker_alarm_lead_s: %v", p.batched)
	}
	b.SweepQueuedBlockers(tk, 1400)
	b.SweepQueuedBlockers(tk, 9000)
	if len(p.batched) != 2 || !strings.HasPrefix(p.batched[1], "lead: T") {
		t.Fatalf("%v", p.batched)
	}
	if n := countEvents(t, tk, "a", "queued_blocker_alarm"); n != 2 || len(p.sent) != 0 {
		t.Fatalf("events %d, typed at once %v", n, p.sent)
	}
	// the wait ends and a new one starts: told again
	if _, err := tk.Q.Exec("UPDATE tasks SET done_evidence = 'x' WHERE id = ?", waiter); err != nil {
		t.Fatal(err)
	}
	b.SweepQueuedBlockers(tk, 9001)
	other := newTask(t, tk, "c", "boss", 0)
	if err := db.AddBlock(tk.Q, other, blocker); err != nil {
		t.Fatal(err)
	}
	tk.Q.Exec("UPDATE task_blocks SET created_at = 9000 WHERE task_id = ?", other)
	b.SweepQueuedBlockers(tk, 9100)
	if len(p.batched) != 3 || !strings.Contains(p.batched[2], fmt.Sprintf("T%d has waited", other)) {
		t.Fatalf("a new wait is told anew: %v", p.batched)
	}
	b.SweepQueuedBlockers(tk, 9400)
	if len(p.batched) != 4 || !strings.HasPrefix(p.batched[3], "lead: ") {
		t.Fatalf("and its lead too: %v", p.batched)
	}
	rows, _ := tk.Q.Query("SELECT json_extract(payload, '$.told') FROM events WHERE kind = 'queued_blocker_alarm' ORDER BY id")
	var told []string
	for rows.Next() {
		var who string
		rows.Scan(&who)
		told = append(told, who)
	}
	rows.Close()
	if strings.Join(told, ",") != "a,lead,a,lead" {
		t.Fatalf("each event names whom it told: %v", told)
	}
}

func TestAReopenedTaskOthersStillWaitOnIsToldAgainCountingFromTheReopen(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.alarmOn, p.boss = true, "lead"
	blocker, _ := queuedBlocker(t, tk)
	b.SweepQueuedBlockers(tk, 1100)
	b.SweepQueuedBlockers(tk, 1400)
	// closed and reopened between two sweeps, twice: the cursors are still set, and the last reopen starts a new wait
	for _, at := range []int{1500, 2000} {
		if _, err := tk.Q.Exec(`INSERT INTO events(ts, agent, kind, payload) VALUES(?, 'boss', 'reopen_task', ?)`, at, fmt.Sprintf(`{"task_id":%d}`, blocker)); err != nil {
			t.Fatal(err)
		}
	}
	b.SweepQueuedBlockers(tk, 2099)
	if len(p.batched) != 2 {
		t.Fatalf("the reopened wait counts from the reopen: %v", p.batched)
	}
	b.SweepQueuedBlockers(tk, 2100)
	b.SweepQueuedBlockers(tk, 2400)
	if len(p.batched) != 4 || !strings.Contains(p.batched[2], "waited on it for 100s") || !strings.HasPrefix(p.batched[3], "lead: ") {
		t.Fatalf("%v", p.batched)
	}
	if _, err := tk.Q.Exec("UPDATE tasks SET done_evidence = 'x', done_at = 2450 WHERE id = ?", blocker); err != nil {
		t.Fatal(err)
	}
	b.SweepQueuedBlockers(tk, 2500)
	var n int
	tk.Q.QueryRow("SELECT COUNT(*) FROM cursors WHERE name LIKE 'blocker_alarm%'").Scan(&n)
	if n != 0 {
		t.Fatalf("a closed task keeps %d alarm cursors", n)
	}
}

func TestTheEarliestWaiterCountsWhateverItsNumberAndTheSweepRunsTheAlarm(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.alarmOn = true
	blocker, _ := queuedBlocker(t, tk)
	later := newTask(t, tk, "c", "boss", 0)
	db.AddBlock(tk.Q, later, blocker)
	tk.Q.Exec("UPDATE task_blocks SET created_at = 950 WHERE task_id = ?", later)
	check(b.Sweep(tk, 1050))
	if len(p.batched) != 1 || !strings.Contains(p.batched[0], "waited on it for 100s") {
		t.Fatalf("%v", p.batched)
	}
}

func TestOnlyAQueuedAgentTaskThatSomethingWaitsOnAlarms(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(tk *Tick, p *tbPorts, blocker, waiter int64)
	}{
		{"doing", func(tk *Tick, _ *tbPorts, blocker, _ int64) { holdSent(t, tk, "a", blocker) }},
		{"itself waiting", func(tk *Tick, _ *tbPorts, blocker, _ int64) {
			db.AddBlock(tk.Q, blocker, newTask(t, tk, "d", "boss", 0)) // its own blocker, a draft, is the head of the chain
		}},
		{"blocked on itself", func(tk *Tick, _ *tbPorts, blocker, waiter int64) {
			tk.Q.Exec("DELETE FROM task_blocks")
			tk.Q.Exec("INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(?, ?, 1000)", blocker, blocker)
		}},
		{"a draft", func(tk *Tick, _ *tbPorts, blocker, _ int64) { db.SetTaskQueued(tk.Q, blocker, "") }},
		{"the founder's", func(tk *Tick, _ *tbPorts, blocker, _ int64) {
			tk.Q.Exec("UPDATE tasks SET owner = ? WHERE id = ?", actor.OwnerKey, blocker)
		}},
		{"the gate's", func(tk *Tick, _ *tbPorts, blocker, _ int64) {
			tk.Q.Exec("UPDATE tasks SET owner = ? WHERE id = ?", db.Gate, blocker)
		}},
		{"its waiter discarded", func(tk *Tick, _ *tbPorts, _, waiter int64) {
			tk.Q.Exec("UPDATE tasks SET discarded_by = 'x' WHERE id = ?", waiter)
		}},
		{"a dormant block", func(tk *Tick, _ *tbPorts, _, _ int64) { tk.Q.Exec("UPDATE task_blocks SET dormant = 1") }},
	} {
		tk, _, b, p := timebox(t)
		p.alarmOn = true
		blocker, waiter := queuedBlocker(t, tk)
		c.set(tk, p, blocker, waiter)
		b.SweepQueuedBlockers(tk, 9000)
		if len(p.batched) != 0 {
			t.Errorf("%s: %v", c.name, p.batched)
		}
	}
}

func TestAQueuedChildItsParentWaitsOnAlarmsAndNamesEveryWaiterButNeverTellsTheFounder(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.alarmOn = true // no lead: the supervisor is the founder
	parent := newTask(t, tk, "b", "boss", 0)
	child, err := db.NewTask(tk.Q, "child", "a", "boss", parent, db.DefaultPriority, "report", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	db.SetTaskQueued(tk.Q, child, "boss")
	waiter := newTask(t, tk, "c", "boss", 0)
	db.AddBlock(tk.Q, waiter, child)
	db.AddBlock(tk.Q, parent, child) // the parent waits twice: as parent from 1000 and by a block from 1080
	tk.Q.Exec("UPDATE tasks SET created_at = 1000 WHERE id = ?", child)
	tk.Q.Exec("UPDATE task_blocks SET created_at = 1050")
	tk.Q.Exec("UPDATE task_blocks SET created_at = 1080 WHERE task_id = ?", parent)
	b.SweepQueuedBlockers(tk, 1099)
	if len(p.batched) != 0 {
		t.Fatalf("the earliest wait (the child's creation) counts: %v", p.batched)
	}
	b.SweepQueuedBlockers(tk, 1100)
	if len(p.batched) != 1 || !strings.Contains(p.batched[0], "waited on it for 100s") {
		t.Fatalf("the parent waits from the child's creation: %v", p.batched)
	}
	b.SweepQueuedBlockers(tk, 9000)
	if len(p.batched) != 1 || !strings.Contains(p.batched[0], fmt.Sprintf("T%d and T%d have waited", parent, waiter)) {
		t.Fatalf("%v", p.batched)
	}
}

// holdSent makes tid sent to agent and the task it holds.
func holdSent(t *testing.T, tk *Tick, agent string, tid int64) {
	t.Helper()
	if _, err := tk.Q.Exec("UPDATE tasks SET sent_at = 1 WHERE id = ?", tid); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCurrentTask(tk.Q, agent, tid, true); err != nil {
		t.Fatal(err)
	}
}

func TestOneWaitIsToldOnceThoughItsFirstWaiterEndsOrTheTaskIsBegunAndPutBack(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.alarmOn = true
	blocker, first := queuedBlocker(t, tk)
	second := newTask(t, tk, "c", "boss", 0)
	db.AddBlock(tk.Q, second, blocker)
	tk.Q.Exec("UPDATE task_blocks SET created_at = 1050 WHERE task_id = ?", second)
	b.SweepQueuedBlockers(tk, 1100)
	tk.Q.Exec("UPDATE tasks SET done_evidence = 'x' WHERE id = ?", first)
	b.SweepQueuedBlockers(tk, 1200)
	holdSent(t, tk, "a", blocker)
	b.SweepQueuedBlockers(tk, 1250)
	db.SetCurrentTask(tk.Q, "a", 0, false)
	b.SweepQueuedBlockers(tk, 1300)
	if len(p.batched) != 1 {
		t.Fatalf("one wait, one notice: %v", p.batched)
	}
}

// The overdue notice and the session-closed ask name what waits behind the held task: the tasks that wait on it, then the holder's queued
// tasks, those others wait on first, three at most.
func TestTheHolderIsToldWhatWaitsBehindItsTask(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 100)
	p.doing[tid] = true
	check(db.SetCurrentTask(tk.Q, "a", tid, true))
	b.SweepTaskTimeouts(tk, 1000)
	b.SweepTaskTimeouts(tk, 1200)
	if len(p.batched) != 1 || strings.Contains(p.batched[0], "Behind it") {
		t.Fatalf("nothing behind, no line: %v", p.batched)
	}
	onIt := newTask(t, tk, "b", "boss", 0)
	check(db.AddBlock(tk.Q, onIt, tid))
	plain := newTask(t, tk, "a", "boss", 0)
	waited := newTask(t, tk, "a", "boss", 0)
	for _, q := range []int64{plain, waited} {
		if _, err := db.SetTaskQueued(tk.Q, q, "boss"); err != nil {
			t.Fatal(err)
		}
	}
	w1, w2 := newTask(t, tk, "c", "boss", 0), newTask(t, tk, "d", "boss", 0)
	check(db.AddBlock(tk.Q, w1, waited))
	check(db.AddBlock(tk.Q, w2, waited))
	newTask(t, tk, "a", "boss", 0) // a draft is not behind it
	want := fmt.Sprintf(" Behind it: T%d waits on it; T%d (queued), which T%d and T%d wait on; T%d (queued). Do it", onIt, waited, w1, w2, plain)
	if got := behindLine(tk, "a", tid); got+" Do it" != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
	extra := newTask(t, tk, "a", "boss", 0)
	if _, err := db.SetTaskQueued(tk.Q, extra, "boss"); err != nil {
		t.Fatal(err)
	}
	if got := behindLine(tk, "a", tid); !strings.HasSuffix(got, fmt.Sprintf("T%d (queued); +1 more.", plain)) {
		t.Fatalf("three named: %q", got)
	}
	check(db.AddBlock(tk.Q, extra, tid))
	if got := behindLine(tk, "a", tid); strings.Count(got, fmt.Sprintf("T%d ", extra)) != 1 || strings.Contains(got, "more") {
		t.Fatalf("a queued task that waits on it is named once: %q", got)
	}
	tk.Q.Exec("DELETE FROM task_blocks WHERE task_id = ?", extra)
	b.SweepTaskTimeouts(tk, 1400)
	if len(p.batched) != 2 || !strings.Contains(p.batched[1], "timeout. Behind it: T") {
		t.Fatalf("%v", p.batched)
	}
	check(db.SetAgentState(tk.Q, "a", states.Idle, 0, ""))
	tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", db.Now()-10)
	if _, err := tk.Q.Exec("DELETE FROM task_blocks WHERE task_id = ?", onIt); err != nil { // what waited on it stops waiting
		t.Fatal(err)
	}
	b.SweepHeldTaskAfterClose(tk, 1500)
	if len(p.said) != 1 || !strings.Contains(p.said[0], fmt.Sprintf("was closed. Behind it: T%d (queued), which", waited)) ||
		!strings.Contains(p.said[0], fmt.Sprintf("T%d (queued). Do it", extra)) {
		t.Fatalf("%v", p.said)
	}
}

func TestManyWaitersAreNamedThreeThenCounted(t *testing.T) {
	tk, _, _, _ := timebox(t)
	tid := newTask(t, tk, "a", "boss", 0)
	var ids []int64
	for range 5 {
		w := newTask(t, tk, "b", "boss", 0)
		check(db.AddBlock(tk.Q, w, tid))
		ids = append(ids, w)
	}
	want := fmt.Sprintf(" Behind it: T%d, T%d, T%d and 2 more wait on it.", ids[0], ids[1], ids[2])
	if got := behindLine(tk, "a", tid); got != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
}

func TestADiscardedTaskDropsItsAlarmCursors(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.alarmOn, p.boss = true, "lead"
	blocker, _ := queuedBlocker(t, tk)
	b.SweepQueuedBlockers(tk, 1400)
	if _, err := tk.Q.Exec("UPDATE tasks SET discarded_by = 'boss' WHERE id = ?", blocker); err != nil {
		t.Fatal(err)
	}
	b.SweepQueuedBlockers(tk, 1500)
	var n int
	tk.Q.QueryRow("SELECT COUNT(*) FROM cursors WHERE name LIKE 'blocker_alarm%'").Scan(&n)
	if len(p.batched) != 2 || n != 0 {
		t.Fatalf("told %v, %d cursors left", p.batched, n)
	}
}

// A task the founder's turn is on top of is the founder's to act on: its time box tells no agent and stands still until the turn ends.
func TestATaskUnderTheFoundersTurnRemindsNoAgent(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.boss = "lead"
	tid := newTask(t, tk, "a", "boss", 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	check(db.PushTurn(tk.Q, tid, db.Turn{Agent: actor.OwnerKey, By: "a", Since: 1010}))
	b.SweepTaskTimeouts(tk, 1010)
	b.SweepTaskTimeouts(tk, 1200)
	if len(p.batched) != 0 {
		t.Fatalf("%v", p.batched)
	}
	var at, left sql.NullInt64
	check(tk.Q.QueryRow("SELECT timeout_at, timeout_left FROM tasks WHERE id = ?", tid).Scan(&at, &left))
	if at.Valid || left.Int64 != 90 {
		t.Fatalf("its clock keeps the 90s it had left for when the turn ends: %v %v", at, left)
	}
}

// toldHolderThenLead checks that the digest got one notice for holder, about it, then one for lead.
func toldHolderThenLead(t *testing.T, p *tbPorts, holder, lead string) {
	t.Helper()
	if len(p.batched) != 2 || !strings.HasPrefix(p.batched[0], holder+": ") || !strings.HasPrefix(p.batched[1], lead+": ") || !strings.Contains(p.batched[0], "held by "+holder) {
		t.Fatalf("%v", p.batched)
	}
}

// A task owned by o with a on top is a's to act on: past its time box a and a's lead are told, not o or o's lead.
func TestATaskUnderAnAgentsTurnTellsThatAgentAndItsLead(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.bosses = map[string]string{"a": "alead", "o": "olead"}
	tid := newTask(t, tk, "o", "boss", 100)
	check(db.PushTurn(tk.Q, tid, db.Turn{Agent: "a", By: "o", Since: 900}))
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	b.SweepTaskTimeouts(tk, 1200)
	toldHolderThenLead(t, p, "a", "alead")
}

// The blocker alarm is about whoever acts on the queued task: an agent on top is told, then its lead, whoever owns it.
func TestTheBlockerAlarmTellsTheAgentOnTop(t *testing.T) {
	tk, _, b, p := timebox(t)
	p.alarmOn = true
	p.bosses = map[string]string{"a": "alead", "c": "clead"}
	blocker, _ := queuedBlocker(t, tk)
	check(db.PushTurn(tk.Q, blocker, db.Turn{Agent: "c", By: "a", Since: 900}))
	b.SweepQueuedBlockers(tk, 1100)
	b.SweepQueuedBlockers(tk, 1400)
	toldHolderThenLead(t, p, "c", "clead")
	// a task the founder owns with an agent on top alarms that agent
	tk2, _, b2, p2 := timebox(t)
	p2.alarmOn = true
	blocker2, _ := queuedBlocker(t, tk2)
	_, err := tk2.Q.Exec("UPDATE tasks SET owner = ? WHERE id = ?", actor.OwnerKey, blocker2)
	check(err)
	check(db.PushTurn(tk2.Q, blocker2, db.Turn{Agent: "c", By: actor.OwnerKey, Since: 900}))
	b2.SweepQueuedBlockers(tk2, 1100)
	if len(p2.batched) != 1 || !strings.HasPrefix(p2.batched[0], "c: ") {
		t.Fatalf("%v", p2.batched)
	}
}

// An agent closed while it holds a doing task it acts on by a turn over another owner is asked to act on it.
func TestAClosedAgentHoldingATaskByATurnIsAsked(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "o", "boss", 100)
	check(db.PushTurn(tk.Q, tid, db.Turn{Agent: "a", By: "o", Since: 900}))
	p.doing[tid] = true
	check(db.SetCurrentTask(tk.Q, "a", tid, true))
	check(db.SetAgentState(tk.Q, "a", states.Idle, 0, ""))
	tk.Q.Exec("UPDATE agent_state SET closed_at = ? WHERE agent = 'a'", int64(500))
	b.SweepHeldTaskAfterClose(tk, 1000)
	if len(p.said) != 1 || !strings.HasPrefix(p.said[0], fmt.Sprintf("a: You hold T%d", tid)) {
		t.Fatalf("%v", p.said)
	}
}

// An agent parked at its account's limit runs no clock of its tasks: it goes on from what it had left when the agent wakes.
func TestAnAgentAtItsLimitStopsItsTasksClock(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	setState(t, tk, "a", states.Parked, 9_000)
	b.SweepTaskTimeouts(tk, 1030)
	b.SweepTaskTimeouts(tk, 5000)
	setState(t, tk, "a", states.Running, 0)
	b.SweepTaskTimeouts(tk, 5000)
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Int64 != 5070 || len(p.batched) != 0 {
		t.Fatalf("the limit ran the clock: %+v %v", task, p.batched)
	}
}

// A clock over its length that stops keeps how far over it was, and a new length (an edit of the timeout) is a new box, with no notice yet.
func TestAnOverClockKeepsItsOverrunAcrossAWaitAndANewLengthStartsANewBox(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	b.SweepTaskTimeouts(tk, 1120)
	p.doing[tid] = false
	b.SweepTaskTimeouts(tk, 1130)
	var left sql.NullInt64
	check(tk.Q.QueryRow("SELECT timeout_left FROM tasks WHERE id = ?", tid).Scan(&left))
	if left.Int64 != -30 || len(p.batched) != 1 {
		t.Fatalf("paused over by 30s: %v %v", left, p.batched)
	}
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 2000)
	if task, _ := db.GetTask(tk.Q, tid); task.TimeoutAt.Int64 != 1970 || must(db.Overdue(tk.Q, task, 2000)) != true {
		t.Fatalf("resumed over by 30s, told: %+v", task)
	}
	secs := int64(600)
	check(db.EditTask(tk.Q, tid, "a", db.TaskEdit{TimeoutS: &secs, IsDoing: true}))
	must(db.AddEvent(tk.Q, "a", "edit_task", db.KV{K: "task_id", V: tid}, db.KV{K: "timeout", V: map[string]any{"old": "100s", "new": "10m"}}))
	task, _ := db.GetTask(tk.Q, tid)
	if n, err := db.TimeoutNotices(tk.Q, tid); err != nil || n.N != 0 || must(db.Overdue(tk.Q, task, 2000)) {
		t.Fatalf("a new length keeps the old box's notices: %d %v", n, err)
	}
}

// An open ask of the founder holds the clock still, as the founder's turn does.
func TestAnOpenAskOfTheFounderStopsTheClock(t *testing.T) {
	tk, _, b, p := timebox(t)
	tid := newTask(t, tk, "a", "boss", 100)
	p.doing[tid] = true
	b.SweepTaskTimeouts(tk, 1000)
	must(tk.Q.Exec("INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(900, 1020, 'a', 'p1', 'which?', 'queued', 'ask')"))
	must(tk.Q.Exec("INSERT INTO task_requests(task_id, request_id) VALUES(?, 900)", tid))
	b.SweepTaskTimeouts(tk, 1020)
	b.SweepTaskTimeouts(tk, 5000)
	var left sql.NullInt64
	check(tk.Q.QueryRow("SELECT timeout_left FROM tasks WHERE id = ?", tid).Scan(&left))
	if left.Int64 != 80 || len(p.batched) != 0 {
		t.Fatalf("the ask ran the clock: %v %v", left, p.batched)
	}
}
