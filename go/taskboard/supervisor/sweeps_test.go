package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

// carolLeadsBob registers carol beside bob.
func (r *rig) carolLeadsBob() { r.register("carol") }

func TestSweepHoldsRecordsWhyEachTaskWaitsAndWarnsOnceWhenNothingIsInTheWay(t *testing.T) {
	r := newRig(t)
	r.carolLeadsBob()
	r.agentUp(r.k("bob"))
	a := r.newTask(r.k("bob"), r.k(("alice")), "first")
	b := r.newTask(r.k("bob"), r.k(("alice")), "second")
	db.SetCurrentTask(r.DB, r.k(("bob")), a, true)
	r.must(r.SweepHolds(r.clock))
	raw, _, _ := db.Meta(r.DB, "taskhold:1")
	if r.named(raw) != `{"why": "nothing is in the way - the daemon sends it on its next tick", "on": "owner", "able": true, "since": 100000, "able_since": 100000}` {
		t.Fatalf("%s", raw)
	}
	raw, _, _ = db.Meta(r.DB, "taskhold:2")
	if r.named(raw) != `{"why": "behind T1, which bob holds (still open)", "on": "owner", "able": false, "since": 100000}` {
		t.Fatalf("%s", raw)
	}
	// blocked
	c := r.newTask(r.k("alice"), r.k(("alice")), "dep")
	db.AddBlock(r.DB, b, c)
	r.clock += 10
	r.must(r.SweepHolds(r.clock))
	raw, _, _ = db.Meta(r.DB, "taskhold:2")
	if r.named(raw) != `{"why": "blocked by T3 (alice)", "on": "task", "able": false, "since": 100010}` {
		t.Fatalf("%s", raw)
	}
	// able for five minutes: each task's opener is told once, an opener that holds it itself leaves it to the founder's warnings
	r.clock += AbleWarnS
	r.must(r.SweepHolds(r.clock))
	r.must(r.SweepHolds(r.clock + 1))
	var warned, toLead int
	r.DB.QueryRow(r.sql("SELECT COUNT(*) FROM requests WHERE target = 'alice' AND body LIKE '%missed delivery%'")).Scan(&warned)
	r.DB.QueryRow(r.sql("SELECT COUNT(*) FROM requests WHERE target = 'carol' AND body LIKE '%missed delivery%'")).Scan(&toLead)
	if warned != 1 || toLead != 0 || !r.logged("T1: waited 5 min for an owner that looks able - alice told") { // alice opened T1 for bob, and T3 for herself
		t.Fatalf("%d %d %v", warned, toLead, r.logs)
	}
	if _, ok, _ := db.Meta(r.DB, db.FounderNoticePrefix+"T3"); !ok {
		t.Fatal("T3 waits on alice, who opened it: the founder is told")
	}
	var body string
	r.DB.QueryRow(r.sql("SELECT body FROM requests WHERE target = 'alice' AND kind = 'say' AND notice >= 1")).Scan(&body)
	if !strings.HasPrefix(r.named(body), "T1 has waited 5 min for bob, which looks able to take it (nothing is in the way") {
		t.Fatalf("%q", body)
	}
	// a finished task's line goes
	db.MarkTaskDone(r.DB, a, "ok", "reason")
	r.must(r.SweepHolds(r.clock + 2))
	if _, found, _ := db.Meta(r.DB, "taskhold:1"); found {
		t.Fatal("a done task still has a line")
	}
}

// Only a task waiting for delivery to an able owner is a missed delivery; a task that waits on purpose never raises the alarm.
func TestTheMissedDeliveryAlarmSkipsTasksThatWaitOnPurpose(t *testing.T) {
	exec := func(r *rig, query string, args ...any) {
		_, err := r.DB.Exec(query, args...)
		r.must(err)
	}
	for _, c := range []struct {
		name  string
		wait  func(r *rig, tid int64)
		alarm bool
	}{
		{"waiting for delivery to an able owner", func(*rig, int64) {}, true},
		{"blocked by a task", func(r *rig, tid int64) { db.AddBlock(r.DB, tid, r.newTask(r.k("alice"), r.k(("alice")), "dep")) }, false},
		{"waiting on its open child", func(r *rig, tid int64) {
			_, err := db.NewTask(r.DB, "child", r.k(("alice")), r.k(("bob")), tid, db.DefaultPriority, "", 0, "")
			r.must(err)
		}, false},
		{"paused, waiting on a human", func(r *rig, tid int64) {
			db.AddBlock(r.DB, tid, r.newTask(actor.OwnerKey, r.k(("bob")), "decide"))
			db.AdvanceTaskTimeout(r.DB, tid, r.clock, 600)
			db.PauseTaskTimeout(r.DB, tid, r.clock)
		}, false},
		{"a draft", func(r *rig, tid int64) {
			exec(r, "UPDATE tasks SET queued_by = NULL, queued_at = NULL WHERE id = ?", tid)
		}, false},
		{"owned by the founder", func(r *rig, tid int64) { exec(r, "UPDATE tasks SET owner = 'human' WHERE id = ?", tid) }, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			r.carolLeadsBob()
			r.agentUp(r.k("bob"))
			tid := r.newTask(r.k("bob"), r.k(("alice")), "job")
			c.wait(r, tid)
			r.must(r.SweepHolds(r.clock))
			r.must(r.SweepHolds(r.clock + AbleWarnS))
			var warned int
			r.DB.QueryRow(r.sql("SELECT COUNT(*) FROM requests WHERE target = 'alice' AND body LIKE ?"), fmt.Sprintf("T%d has waited %%", tid)).Scan(&warned)
			if (warned == 1) != c.alarm || warned > 1 {
				t.Fatalf("alarm %v, %d told", c.alarm, warned)
			}
		})
	}
}

func TestAWaitOnTheOwnerIsFiledUnderItsAccountsWallOrCap(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	if c := r.ownerCause(r.DB, r.k(("bob"))); c != "owner" {
		t.Fatalf("nothing holds the account: %q", c)
	}
	r.life.wait = "cap"
	if c := r.ownerCause(r.DB, r.k(("bob"))); c != "cap" {
		t.Fatalf("no free slot: %q", c)
	}
	r.Waits = fakeWaits{limit: map[string]bool{r.k(("bob")): true}}
	if c := r.ownerCause(r.DB, r.k(("bob"))); c != "wall" {
		t.Fatalf("at its usage limit: %q", c)
	}
}

func TestAgentReasonNamesTheFirstGateInTheOrderItIsMet(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	if why, able := r.agentReason(r.DB, r.k(("bob")), r.clock); why != "" || !able {
		t.Fatalf("%q %v", why, able)
	}
	r.NoteHold(r.k("bob"), "its screen is not idle (mid-turn, or unreadable)", true)
	if why, able := r.agentReason(r.DB, r.k(("bob")), r.clock); why != "its screen is not idle (mid-turn, or unreadable)" || !able {
		t.Fatalf("%q %v", why, able)
	}
	db.SetAgentState(r.DB, r.k(("bob")), "RUNNING", 0, "")
	if _, able := r.agentReason(r.DB, r.k(("bob")), r.clock); able {
		t.Fatal("a contestable screen reading is contested only when the state says idle")
	}
	db.SetCursor(r.DB, "clear_owed:"+r.k("bob"), 5)
	if why, _ := r.agentReason(r.DB, r.k(("bob")), r.clock); !strings.Contains(r.named(why), "owes a context clear first") {
		t.Fatal(why)
	}
	r.Stuck = fakeStuck{why: "bob is mid-turn - nothing is typed at a working agent"}
	if why, _ := r.agentReason(r.DB, r.k(("bob")), r.clock); !strings.Contains(r.named(why), "mid-turn") {
		t.Fatal(why)
	}
	db.BlockOpen(r.DB, r.k(("bob")), "dialog", "trust?", "w", 1, 1, "tick", "t")
	if why, _ := r.agentReason(r.DB, r.k(("bob")), r.clock); why != "dialog: trust?" {
		t.Fatal(why)
	}
	r.NoteFailure(r.k("bob"), "the wake failed")
	if why, _ := r.agentReason(r.DB, r.k(("bob")), r.clock); why != "the wake failed" {
		t.Fatal(why)
	}
	if why, _ := r.agentReason(r.DB, r.k(("bob")), r.clock+HeldFreshS+1); why == "the wake failed" {
		t.Fatal("a stale failure is still told")
	}
}

func TestABusyAgentPastTheWindowGetsNoDigestUntilItsTurnEndsThenOne(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	for _, body := range []string{"T5 done by alice: it works", "T6 discarded by alice: not needed", "something else"} {
		db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: r.k(("bob")), Body: body, Kind: "say", Notice: true})
	}
	db.SetAgentState(r.DB, r.k(("bob")), "RUNNING", 0, "")
	for _, at := range []int64{10, 900, 5000} {
		r.must(r.SweepDigests(r.clock + at))
		if len(r.term.typed) != 0 {
			t.Fatalf("typed to a working agent %ds after its first notice", at)
		}
	}
	db.SetAgentState(r.DB, r.k(("bob")), "IDLE", 0, "") // the turn ended
	r.must(r.SweepDigests(r.clock + 5030))
	r.must(r.SweepDigests(r.clock + 5060))
	if len(r.term.typed) != 1 {
		t.Fatalf("one digest at the turn's end: %q", r.term.typed)
	}
	want := "Result digest - 3 results since "
	got := r.term.last()
	if !strings.Contains(got, want) || !strings.Contains(r.named(got), "shows one whole:\nT5 · alice · it works (#1)\nT6 · alice · discarded: not needed (#2)\nsomething else (#3)\n") {
		t.Fatalf("%q", got)
	}
	if left, _ := db.PendingNotices(r.DB, r.k(("bob"))); len(left) != 0 {
		t.Fatal("digested notices stay pending")
	}
}

func TestAnIdleAgentGetsItsDigestAtTheWindowNotBefore(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: r.k(("bob")), Body: "T9 done by alice: ok", Kind: "say", Notice: true})
	r.must(r.SweepDigests(r.clock + 899))
	if len(r.term.typed) != 0 {
		t.Fatalf("typed before the window ran: %q", r.term.typed)
	}
	r.must(r.SweepDigests(r.clock + 900))
	if !strings.Contains(r.term.last(), "Result digest - 1 result since ") {
		t.Fatalf("%q", r.term.last())
	}
}

func TestAnAgentAtItsPromptWithBackgroundWorkStillGetsItsDigest(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	r.must(db.SetCursor(r.DB, "bgwork:"+r.k("bob"), 1))
	db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: r.k(("bob")), Body: "T9 done by alice: ok", Kind: "say", Notice: true})
	r.must(r.SweepDigests(r.clock + 900))
	if !strings.Contains(r.term.last(), "Result digest - 1 result since ") {
		t.Fatalf("the digest waited on background work: %q", r.term.typed)
	}
}

func TestDigestTextCapsItsLines(t *testing.T) {
	var rows []*db.Request
	for i := 0; i < 23; i++ {
		rows = append(rows, &db.Request{ID: int64(i + 1), Ref: fmt.Sprintf("%012x", i+1), Body: "T1 done by a: " + strings.Repeat("x", 100)})
		rows[i].LandedAt.Valid, rows[i].LandedAt.Int64 = true, 60
	}
	text := DigestText(newRig(t).DB, rows)
	lines := strings.Split(text, "\n")
	if len(lines) != 1+20+1 || lines[21] != "(+3 more: `hl inbox --all`)" {
		t.Fatalf("%d %q", len(lines), lines[21])
	}
	if !strings.HasSuffix(strings.Split(lines[1], " · ")[2], "… (msg_0000000)") {
		t.Fatalf("%q", lines[1])
	}
}

func TestStoppedAgentsAreToldToWhoeverWaitsOnThemOncePerSpell(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("alice"))
	r.agentUp(r.k("bob"))
	r.newTask(r.k("bob"), r.k(("alice")), "bob's job")
	db.SetStalled(r.DB, r.k(("bob")), "send_dialog", "cannot get past a dialog: Trust?", nil)
	r.must(r.SweepStopped(r.clock))
	if got := r.term.last(); !strings.Contains(r.named(got), "bob is stalled: cannot get past a dialog: Trust?") {
		t.Fatalf("%q", got)
	}
	r.term.typed = nil
	r.must(r.SweepStopped(r.clock + 1))
	if len(r.term.typed) != 0 {
		t.Fatal("told twice in one spell")
	}
	db.SetAgentState(r.DB, r.k(("bob")), "IDLE", 0, "")
	r.must(r.SweepStopped(r.clock + 2))
	db.SetAgentState(r.DB, r.k(("bob")), "STALLED", 0, "gone wrong")
	r.must(r.SweepStopped(r.clock + 3))
	if got := r.term.last(); !strings.Contains(r.named(got), "bob is FATAL: gone wrong") {
		t.Fatalf("a new spell is told afresh: %q", got)
	}
}

func TestLandingSweepConfirmsFindsAndRetypes(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	db.SetAccount(r.DB, r.k(("bob")), "acct", "recorded")
	tr := filepath.Join(r.home, "s1.jsonl")
	r.sessions.transcripts = map[string]string{"s1": tr}
	os.WriteFile(tr, nil, 0o644)
	rid := r.ask(r.k("alice"), r.k(("bob")), "typed but unseen", "ask")
	r.TellNow(r.DB, rid)
	// the transcript does not show it yet and it was typed a moment ago: wait
	r.clock += 10
	r.must(r.LandingSweep(r.DB, r.k(("bob"))))
	if row, _ := db.GetRequest(r.DB, rid); row.State != "queued" || !row.TypedAt.Valid {
		t.Fatalf("%+v", row)
	}
	// past the wait it is owed again
	r.clock += LandingWaitS
	r.must(r.LandingSweep(r.DB, r.k(("bob"))))
	row, _ := db.GetRequest(r.DB, rid)
	if row.TypedAt.Valid || row.DeliveredAt.Valid {
		t.Fatalf("not owed again: %+v", row)
	}
	if !r.logged("request 1 never reached its transcript - typed again") {
		t.Fatal(r.logs)
	}
	// it lands
	r.TellNow(r.DB, rid)
	row, _ = db.GetRequest(r.DB, rid)
	line := `{"type":"queue-operation","operation":"enqueue","content":"` + row.Name(r.DB) + ` from alice, waiting for you:\ntyped but unseen"}` + "\n"
	os.WriteFile(tr, []byte(line), 0o644)
	r.must(r.LandingSweep(r.DB, r.k(("bob"))))
	if row, _ := db.GetRequest(r.DB, rid); row.State != "landed" || !row.LandedAt.Valid {
		t.Fatalf("%+v", row)
	}
}

func TestLandingTakesATypedLineAsLandedWhereTheVendorKeepsNoTranscript(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	db.SetAccount(r.DB, r.k(("bob")), "acct", "recorded")
	rid := r.ask(r.k("alice"), r.k(("bob")), "x", "ask")
	r.TellNow(r.DB, rid)
	r.must(r.LandingSweep(r.DB, r.k(("bob"))))
	if row, _ := db.GetRequest(r.DB, rid); row.State != "landed" {
		t.Fatalf("%+v", row)
	}
}

func TestLandingStateNamesWhereARequestIs(t *testing.T) {
	var row db.Request
	if LandingState(&row) != "queued" {
		t.Fatal()
	}
	row.TypedAt.Valid, row.TypedAt.Int64 = true, 5
	if LandingState(&row) != "typed" {
		t.Fatal()
	}
	row.LandedAt.Valid, row.LandedAt.Int64 = true, 6
	if LandingState(&row) != "delivered" {
		t.Fatal()
	}
}

func TestAHeldMemoWaitsForTheSameWindowAsAnyNotice(t *testing.T) {
	r := newRig(t)
	r.agentUp(r.k("bob"))
	db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: r.k(("bob")), Body: "T4 comment by alice: see the log", Kind: "say", Memo: true})
	r.must(r.SweepDigests(r.clock + 10))
	if len(r.term.typed) != 0 {
		t.Fatalf("a memo's tell skipped the window: %q", r.term.typed)
	}
	r.must(r.SweepDigests(r.clock + 900))
	if !strings.Contains(r.term.last(), "T4 comment by alice: see the log") {
		t.Fatalf("%q", r.term.last())
	}
}

// A gate is held by the daemon, not waiting for an agent: the hold sweep keeps no reason for it, so it reads as watching.
func TestTheHoldSweepLeavesAGateAlone(t *testing.T) {
	r := newRig(t)
	_, err := r.DB.Exec(r.sql("INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by, sent_at, watch_vendor, watch_target, watch_until, ") +
		"watch_label) VALUES(1, 'wait for CI', 'gate', 'bob', 1, 'bob', 1, 'github', 'o/r#7', 'ci', 'PR #7 CI')")
	r.must(err)
	r.must(r.SweepHolds(r.clock))
	if raw, _, _ := db.Meta(r.DB, "taskhold:1"); raw != "" {
		t.Fatalf("a gate is held: %s", raw)
	}
}

// A held notice in the digest keeps what happened, how long and what waits behind whole; only the title is cut, and what to do is left out.
func TestADigestLineCutsAHeldNoticesTitleNeverItsFacts(t *testing.T) {
	title := strings.Repeat("a long title ", 6)
	for _, c := range []struct{ text, want string }{
		{"T12 (" + title + "), held by handline.ui, has been doing past its 4h timeout. Behind it: T13 (queued), which T14 waits on. Do it, hand it on.",
			"has been doing past its 4h timeout. Behind it: T13 (queued), which T14 waits on. (msg_x)"},
		{"T12 (" + title + "), held by a, is queued, and T2 has waited on it for 2h. Start it, close it if its work is done.",
			"held by a, is queued, and T2 has waited on it for 2h. (msg_x)"},
		{"T12 (short), held by a, has been doing past its 90m timeout. Do it.", "T12 (short), held by a, has been doing past its 90m timeout. (msg_x)"},
		{"T12 (v1.2 ), held by b, x), held by a, has been doing past its 90m timeout. Do it.",
			"T12 (v1.2 ), held by b, x), held by a, has been doing past its 90m timeout. (msg_x)"},
	} {
		got := digestSummary(c.text, "msg_x")
		if !strings.HasSuffix(got, c.want) || !strings.HasPrefix(got, "T12 (") {
			t.Errorf("%q\nwant the end %q", got, c.want)
		}
		if strings.Contains(c.text, title) && !strings.Contains(got, "…") {
			t.Errorf("the title is cut: %q", got)
		}
	}
	// a cut that the facts leave room for fills the line exactly
	got := digestSummary("T12 ("+title+"), held by a, is queued, and T2 has waited on it for 2h. Start it.", "msg_x")
	if n := len([]rune(strings.TrimSuffix(got, " (msg_x)"))); n != ResultShown+30 && n != ResultShown+29 { // a cut on a space drops it
		t.Errorf("%d runes: %q", n, got)
	}
	// facts that leave no room still keep TitleMin of the title
	long := "T12 (" + strings.Repeat("abcdefghij", 4) + "), held by a, has been doing past its 4h timeout. Behind it: T13 (queued), which T14, T15, T16 " +
		"and 2 more wait on; T17 (queued), which T18 waits on; T19 (queued); +2 more. Do it."
	if got := digestSummary(long, "msg_x"); !strings.HasPrefix(got, "T12 (abcdefghijabcdefghij…), held by a,") {
		t.Errorf("%q", got)
	}
	// a cut that lands on a space drops the space before the ellipsis
	facts := "held by a, is queued, and T2 has waited on it for 2h."
	keep := ResultShown + 30 - len(facts) - len("12") - 7
	spaced := "T12 (" + strings.Repeat("x", keep-1) + " yyyyyyyyyy), " + facts + " Start it."
	if got := digestSummary(spaced, "msg_x"); !strings.HasPrefix(got, "T12 ("+strings.Repeat("x", keep-1)+"…), ") {
		t.Errorf("%q", got)
	}
}

// A memo held an hour unread is told once to whoever wrote it, who is the one waiting on it; one the founder wrote tells no one.
func TestAMemoUnreadAnHourIsToldToItsWriterOnce(t *testing.T) {
	r := newRig(t)
	tid := r.newTask(r.k("bob"), r.k("carol"), "job")
	for i, writer := range []string{r.k("alice"), actor.OwnerKey} {
		mid, err := db.AddMemo(r.DB, tid, writer, fmt.Sprintf("note %d", i))
		r.must(err)
		rid, err := db.AddRequest(r.DB, db.NewRequest{Requester: db.NoticeRequester, Target: r.k("bob"), Body: fmt.Sprintf("T1 comment %d", i), Kind: "say", Memo: true})
		r.must(err)
		_, err = r.DB.Exec("UPDATE requests SET state = 'landed', landed_at = 1 WHERE id = ?", rid)
		r.must(err)
		_, err = r.DB.Exec("INSERT INTO memo_tells(memo_id, reader, request_id) VALUES(?, ?, ?)", mid, r.k("bob"), rid)
		r.must(err)
	}
	r.must(r.SweepHeldMemos(MemoLeadS + 1))
	r.must(r.SweepHeldMemos(MemoLeadS + 2))
	var told int
	r.DB.QueryRow("SELECT COUNT(*) FROM requests WHERE target = ? AND body LIKE '%has not read this comment in an hour%'", r.k("alice")).Scan(&told)
	var others int
	r.DB.QueryRow("SELECT COUNT(*) FROM requests WHERE target != ? AND body LIKE '%has not read this comment in an hour%'", r.k("alice")).Scan(&others)
	if told != 1 || others != 0 {
		t.Fatalf("alice is told once, no one else: %d %d", told, others)
	}
}
