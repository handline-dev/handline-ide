package rulesfold

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
)

// A step reads the person's comments and chat past the cursors, skips acks, writes the rules the model returns as lessons, and moves the
// cursors; a step with nothing new asks nothing.
func TestAStepWritesTheRulesAndMovesTheCursors(t *testing.T) {
	d := board(t)
	tid := task(t, d, "Settings tab")
	comment(t, d, tid, "a1", "Here is the settings tab.", 100)
	c1 := comment(t, d, tid, person, "OK", 110)
	c2 := comment(t, d, tid, person, "Too many words on this screen, cut them", 120)
	chat(t, d, "a2", person, "Done, merged.", 130)
	m1 := chat(t, d, person, "a2", "Always tell me what changed before you ask", 140)
	chat(t, d, "a3", person, "note", 150)

	a := &answer{rules: []map[string]any{
		rule(0, "Keep screen copy short.", 1, "Too many words on this screen", "comment "+itoa(c2)),
		rule(0, "Say what changed before asking for a look.", 1, "Always tell me what changed", "msg_"+ref(t, d, m1)),
	}}
	res, err := Step(context.Background(), d, person, a.model, true)
	if err != nil || !res.Written {
		t.Fatal(res, err)
	}
	if len(a.asked) != 1 || strings.Contains(a.asked[0], `"said":"OK"`) || !strings.Contains(a.asked[0], "Here is the settings tab.") ||
		!strings.Contains(a.asked[0], "Done, merged.") {
		t.Fatalf("prompt: the ack must be left out and each item carry what it answered:\n%s", a.asked[0])
	}
	got := live(t, d)
	if len(got) != 2 || got[0].Text != `Keep screen copy short. (said once; "Too many words on this screen", comment `+itoa(c2)+")" ||
		got[0].Task != tid || !strings.HasPrefix(got[1].Text, "Say what changed") || got[1].Task != 0 || got[0].CreatedBy != By {
		t.Fatalf("lessons: %+v", got)
	}
	if cur, _ := ReadCursors(d, person); cur.Comments != c2 || cur.Chat != m1 || c1 >= c2 {
		t.Fatalf("cursors %+v, want comments %d chat %d", cur, c2, m1)
	}
	a.asked = nil
	if res, err := Step(context.Background(), d, person, a.model, true); err != nil || len(a.asked) != 0 || res.Written {
		t.Fatal("a step with nothing new must ask nothing", res, err)
	}
}

// A dry run asks and validates but writes nothing and moves no cursor.
func TestADryRunWritesNothing(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Never show internal ids on screen", 100)
	a := &answer{rules: []map[string]any{rule(0, "Show names, not ids.", 1, "Never show internal ids", "comment "+itoa(c))}}
	res, err := Step(context.Background(), d, person, a.model, false)
	if err != nil || res.Written || len(res.Rules) != 1 {
		t.Fatal(res, err)
	}
	if cur, _ := ReadCursors(d, person); cur != (Cursors{}) || len(live(t, d)) != 0 {
		t.Fatal("a dry run wrote", cur)
	}
}

// Nothing done before is lost: an answer that quotes what was not read or returns a rule twice is refused, and the cursors stay; a kept
// rule left out stays as it was, and one said again counts up from what it had, whatever count the model gives. An id the model made up
// is a new rule.
func TestAnAnswerThatLosesARuleIsRefusedAndTheCursorsStay(t *testing.T) {
	d := board(t)
	hand, _ := db.AddLesson(d, person, "Write to the reviewer in Korean.", 0, person)
	twice, _ := db.AddLesson(d, person, `Fix the cause. (said 2 times; "근본수정", comment 5)`, 0, By)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Do not use cards for small things", 100)
	keepBoth := []map[string]any{rule(hand, "Write to the reviewer in Korean.", 1, "", ""),
		rule(twice, "Fix the cause.", 2, "근본수정", "comment 5")}
	for name, rules := range map[string][]map[string]any{
		"a quote not read": append(keepBoth, rule(0, "No cards.", 1, "Do not use cards", "comment 12345")),
		"a rule twice":     append(keepBoth, rule(0, "No cards.", 1, "Do not use cards", "comment "+itoa(c), hand)),
	} {
		a := &answer{rules: rules}
		if _, err := Step(context.Background(), d, person, a.model, true); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if cur, _ := ReadCursors(d, person); cur != (Cursors{}) || len(live(t, d)) != 2 {
			t.Fatalf("%s: wrote something", name)
		}
	}
	again := rule(twice, "Fix the cause.", 1, "근본수정", "comment 5")
	again["refs"] = []string{"comment " + itoa(c), "comment " + itoa(c)}
	a := &answer{rules: []map[string]any{rule(99, "No cards for small things.", 1, "Do not use cards", "comment "+itoa(c), 98), again}}
	if res, err := Step(context.Background(), d, person, a.model, true); err != nil || !res.Written || len(live(t, d)) != 3 {
		t.Fatal(res, err)
	}
	// The person's own rule is kept as it was: an unchanged rule is not rewritten.
	ls := live(t, d)
	if ls[0].ID != hand || !strings.HasPrefix(ls[2].Text, "Fix the cause. (said 3 times") {
		t.Fatalf("%+v", ls)
	}
}

// A rule said again counts up and keeps its id's history; kept rules the model merges become one, and one it leaves out stays as it
// was; a rule merged from a person's keeps that protection.
func TestRulesCountUpMergeAndOneLeftOutStays(t *testing.T) {
	d := board(t)
	hand, _ := db.AddLesson(d, person, "Lists are flat rows.", 0, person)
	once, _ := db.AddLesson(d, person, `No cards. (said once; "카드 쓰지 말자", comment 3)`, 0, By)
	stays, _ := db.AddLesson(d, person, `Merge quietly. (said once; "조용히", comment 4)`, 0, By)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Again cards everywhere, use flat rows", 100)
	a := &answer{rules: []map[string]any{rule(hand, "Lists are flat rows, never cards.", 3, "use flat rows", "comment "+itoa(c), once)}}
	res, err := Step(context.Background(), d, person, a.model, true)
	if err != nil || len(res.Dropped) != 0 {
		t.Fatal(res, err)
	}
	ls := live(t, d)
	if len(ls) != 2 || ls[0].ID != stays || ls[1].Text != `Lists are flat rows, never cards. (said 3 times; "use flat rows", comment `+itoa(c)+")" || ls[1].Task != tid {
		t.Fatalf("%+v", ls)
	}
	var mergedInto int64
	_ = d.QueryRow("SELECT merged_into FROM lessons WHERE id = ?", hand).Scan(&mergedInto)
	if mergedInto != ls[1].ID {
		t.Fatal("the person's rule must be kept, merged into the new one")
	}
	// Merged from a person's rule, it stays protected though the fold wrote it.
	k, _ := keptRules(d, person)
	if k[0].keep || !k[1].keep {
		t.Fatal("a rule merged from a person's must keep its protection")
	}
}

// A lesson added while the model answered (a Redo) leaves the step unwritten, its batch read again next time.
func TestARuleAddedWhileTheModelAnsweredLeavesTheStepUnwritten(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Show before and after pictures", 100)
	a := &answer{rules: []map[string]any{rule(0, "Show pictures.", 1, "Show before and after", "comment "+itoa(c))},
		during: func() { _, _ = db.AddLesson(d, person, "from a Redo", 0, person) }}
	if _, err := Step(context.Background(), d, person, a.model, true); !errors.Is(err, ErrMoved) {
		t.Fatal(err)
	}
	if cur, _ := ReadCursors(d, person); cur != (Cursors{}) || len(live(t, d)) != 1 {
		t.Fatal("wrote past a change", cur)
	}
}

// Past the cap, the fold's least said rules go until it fits; a rule said twice outlasts those said once. A count rises only by the items read that say it. A set
// that cannot fit is refused.
func TestPastTheCapTheLeastSaidDroppableRulesGo(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "This is a rule about screens", 100)
	c2 := comment(t, d, tid, person, "Again: a rule about screens", 110)
	long := strings.Repeat("x", 110)
	twice := rule(0, "Said twice.", 2, "This is a rule", "comment "+itoa(c))
	twice["refs"] = []string{"comment " + itoa(c), "comment " + itoa(c2)}
	rules := []map[string]any{rule(0, "Claimed often.", 9, "This is a rule", "comment "+itoa(c))}
	for i := range 200 {
		rules = append(rules, rule(0, long+itoa(int64(i)), 1, "This is a rule", "comment "+itoa(c)))
	}
	a := &answer{rules: append(rules, twice)}
	res, err := Step(context.Background(), d, person, a.model, true)
	if err != nil {
		t.Fatal(err)
	}
	ls := live(t, d)
	if len(ls) < 2 || len(ls) >= 202 || !db.Fits(person, ls) {
		t.Fatalf("%d rules kept", len(ls))
	}
	var found bool
	for _, r := range res.Rules {
		found = found || r.Text == "Said twice." && r.Count == 2
		if r.Text == "Claimed often." && r.Count != 1 {
			t.Fatalf("a count of 9 with one item read must be 1, got %d", r.Count)
		}
	}
	if !found {
		t.Fatal("a rule said twice must stay past the cap")
	}
	// Protected rules alone past the cap: refused.
	d2 := board(t)
	for i := range 250 {
		_, _ = d2.Exec("INSERT INTO lessons(agent, text, created_at, created_by) VALUES(?, ?, 1, ?)", person, long+itoa(int64(i)), person)
	}
	tid2 := task(t, d2, "x")
	c3 := comment(t, d2, tid2, person, "one more screen rule", 100)
	k, _ := keptRules(d2, person)
	var all []map[string]any
	for _, r := range k {
		all = append(all, rule(r.ID, r.Text, 1, "", ""))
	}
	a2 := &answer{rules: append(all, rule(0, "x", 1, "one more", "comment "+itoa(c3)))}
	if _, err := Step(context.Background(), d2, person, a2.model, true); err == nil {
		t.Fatal("a set past the cap with nothing to drop must be refused")
	}
}

// Due: a backlog is due at once, new words a day after the last fold, nothing new never.
func TestAFoldIsDueForABacklogOrADayLater(t *testing.T) {
	d := board(t)
	now := time.Unix(1_000_000, 0)
	if due, _ := Due(d, person, now); due {
		t.Fatal("nothing to read is not due")
	}
	tid := task(t, d, "x")
	comment(t, d, tid, person, "a standard to keep", 100)
	if due, _ := Due(d, person, now); !due {
		t.Fatal("never folded and something to read: due")
	}
	_ = writeCursors(d, person, Cursors{})
	_, _ = d.Exec("UPDATE cursors SET updated = ?", now.Add(-time.Hour).Unix())
	if due, _ := Due(d, person, now); due {
		t.Fatal("folded an hour ago with a small batch waiting: not due")
	}
	if due, _ := Due(d, person, now.Add(Every)); !due {
		t.Fatal("a day later: due")
	}
	for i := range 60 {
		comment(t, d, tid, person, strings.Repeat("long words ", 80)+itoa(int64(i)), 200+int64(i))
	}
	if due, _ := Due(d, person, now); !due {
		t.Fatal("more than a batch waiting: due at once")
	}
}

// The daily runner reads a backlog a batch after another until it is read, then waits a day; it never runs while it is off.
func TestTheDailyRunnerReadsABacklogThenWaitsADay(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	var refs []string
	for i := range 40 {
		refs = append(refs, "comment "+itoa(comment(t, d, tid, person, strings.Repeat("a lasting standard ", 100)+itoa(int64(i)), 100+int64(i))))
	}
	asked := 0
	model := func(_ context.Context, prompt, _ string) ([]byte, error) {
		asked++
		for _, r := range refs {
			if strings.Contains(prompt, `"ref":"`+r+`"`) {
				return []byte(`{"rules":[{"id":0,"text":"Keep the standard.","count":1,"quote":"a lasting standard","quote_ref":"` + r + `","refs":[]}]}`), nil
			}
		}
		return nil, errors.New("no item")
	}
	now := time.Unix(2_000_000, 0)
	on := false
	daily := &Daily{DB: d, Model: func() (Model, bool) { return model, true }, On: func() bool { return on },
		Log: func(string, ...any) {}, Now: func() time.Time { return now }}
	daily.Check(context.Background())
	if asked != 0 {
		t.Fatal("ran while off")
	}
	on = true
	daily.Check(context.Background())
	for daily.running.Load() {
		time.Sleep(time.Millisecond)
	}
	cur, _ := ReadCursors(d, person)
	if asked < 2 || cur.Comments == 0 {
		t.Fatalf("asked %d, cursor %+v: the backlog must be read in batches", asked, cur)
	}
	if due, _ := Due(d, person, now); due {
		t.Fatal("read to the end: not due again until a day passes")
	}
}

// A person's rule longer than a rule the fold writes, or on two lines, comes back as it was and stands as they wrote it.
func TestAPersonsLongRuleStandsAsWritten(t *testing.T) {
	d := board(t)
	text := strings.Repeat("Ask one item at a time. ", 12) + "\nAnd no hints."
	hand, _ := db.AddLesson(d, person, text, 0, person)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Show before and after pictures", 100)
	a := &answer{rules: []map[string]any{rule(hand, text, 1, "", ""), rule(0, "Show pictures.", 1, "Show before and after", "comment "+itoa(c))}}
	if _, err := Step(context.Background(), d, person, a.model, true); err != nil {
		t.Fatal(err)
	}
	if ls := live(t, d); ls[0].ID != hand || ls[0].Text != text {
		t.Fatalf("%+v", ls[0])
	}
	// said again, it counts up and keeps the person's words, however long
	c2 := comment(t, d, tid, person, "Ask me one item at a time, again", 200)
	pics := live(t, d)[1].ID
	a.rules = []map[string]any{rule(hand, text, 2, "Ask me one item at a time", "comment "+itoa(c2)),
		rule(pics, "Show pictures.", 1, "Show before and after", "comment "+itoa(c))}
	if _, err := Step(context.Background(), d, person, a.model, true); err != nil {
		t.Fatal("a person's long rule said again must not stop the fold: ", err)
	}
	if ls := live(t, d); len(ls) != 2 || !strings.HasPrefix(ls[1].Text, text) || !strings.Contains(ls[1].Text, "said 2 times") {
		t.Fatalf("%+v", ls)
	}
}

// A batch of only acks moves the cursors and leaves every rule as it was: no copies.
func TestABatchOfOnlyAcksLeavesTheRulesAsTheyWere(t *testing.T) {
	d := board(t)
	hand, _ := db.AddLesson(d, person, "Write to the reviewer in Korean.", 0, person)
	tid := task(t, d, "x")
	a := &answer{}
	for i := range 2 {
		c := comment(t, d, tid, person, "OK", int64(100+i))
		if res, err := Step(context.Background(), d, person, a.model, true); err != nil || !res.Written {
			t.Fatal(res, err)
		}
		if cur, _ := ReadCursors(d, person); cur.Comments != c {
			t.Fatal(cur)
		}
	}
	if ls := live(t, d); len(a.asked) != 0 || len(ls) != 1 || ls[0].ID != hand || ls[0].Text != "Write to the reviewer in Korean." {
		t.Fatalf("asked %d, lessons %+v", len(a.asked), ls)
	}
}

// A step that fails waits a day before the next try, even with a backlog: no hourly retry.
func TestAFailedFoldWaitsADay(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	for i := range 60 {
		comment(t, d, tid, person, strings.Repeat("long words ", 80)+itoa(int64(i)), 100+int64(i))
	}
	now := time.Unix(2_000_000, 0)
	asked := 0
	daily := &Daily{DB: d, On: func() bool { return true }, Log: func(string, ...any) {}, Now: func() time.Time { return now },
		Model: func() (Model, bool) {
			return func(context.Context, string, string) ([]byte, error) { asked++; return nil, errors.New("no answer") }, true
		}}
	daily.run(context.Background(), person)
	if asked != 1 {
		t.Fatal(asked)
	}
	if due, _ := Due(d, person, now.Add(time.Hour)); due {
		t.Fatal("an hour after a failure: not due")
	}
	if due, _ := Due(d, person, now.Add(Every)); !due {
		t.Fatal("a day after a failure: due")
	}
}

// handoff records tid passed to agent at ts.
func handoff(t *testing.T, q db.Q, tid int64, to string, ts int64) {
	t.Helper()
	if _, err := q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, 'a0', 'task_handoff', json_object('task_id', ?, 'to', ?))", ts, tid, to); err != nil {
		t.Fatal(err)
	}
}

// A correction of one agent's work becomes that agent's own lesson on the task, and a standard for any agent's work a rule of the
// person's; the item names the agents that held the task up to then, not one it went to after.
func TestACorrectionOfAnAgentsWorkBecomesItsLesson(t *testing.T) {
	d := board(t)
	tid := task(t, d, "Usage page")
	if _, err := d.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(100, 'a0', 'task_new', json_object('task_id', ?, 'owner', 'a1'))", tid); err != nil {
		t.Fatal(err)
	}
	handoff(t, d, tid, "a2", 105)
	for _, e := range [][]any{{106, "a5", "task_begin", "{}"}, {107, "p1", "edit_task", `{"owner": {"old": "a5", "new": "a4"}}`}} {
		if _, err := d.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, ?, ?, json_set(?, '$.task_id', ?))", e[0], e[1], e[2], e[3], tid); err != nil {
			t.Fatal(err)
		}
	}
	c1 := comment(t, d, tid, person, "You did not check it in the running app before asking me", 110)
	c2 := comment(t, d, tid, person, "Every screen must work by keyboard", 120)
	handoff(t, d, tid, "a3", 130)
	a := &answer{
		rules:   []map[string]any{rule(0, "Every screen works by keyboard.", 1, "work by keyboard", "comment "+itoa(c2))},
		lessons: []map[string]any{{"agent": "a2", "text": "Check a screen in the running app before asking the reviewer.", "refs": []string{"comment " + itoa(c1)}}},
	}
	res, err := Step(context.Background(), d, person, a.model, true)
	if err != nil || !res.Written || len(res.Lessons) != 1 || len(res.Skipped) != 0 {
		t.Fatal(res, err)
	}
	if !strings.Contains(a.asked[0], `"agents":["a1","a2","a5","a4"]`) || !strings.Contains(a.asked[0], `LESSONS:`) {
		t.Fatalf("the item must name the agents that held the task then:\n%s", a.asked[0])
	}
	if ls := live(t, d); len(ls) != 1 || !strings.HasPrefix(ls[0].Text, "Every screen works by keyboard.") {
		t.Fatalf("person: %+v", ls)
	}
	own, _ := db.LiveLessons(d, "a2")
	if len(own) != 1 || own[0].Text != "Check a screen in the running app before asking the reviewer." || own[0].Task != tid || own[0].CreatedBy != By {
		t.Fatalf("a2: %+v", own)
	}
	if other, _ := db.LiveLessons(d, "a3"); len(other) != 0 {
		t.Fatal("an agent the task went to after must learn nothing from it")
	}
}

// An agent lesson must come from an item about that agent's work and merge only that agent's live lessons, each once; one that does not
// is left out with why, and the rest of the step written.
func TestAnAgentLessonMustBeAboutThatAgentsWork(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	other, _ := db.AddLesson(d, "a2", "Run the tests.", 0, "a2")
	mine, _ := db.AddLesson(d, "a1", "Run the linter.", 0, "a1")
	for i, l := range []map[string]any{
		{"agent": "a2", "text": "Run the tests."},                                  // an agent the item does not name
		{"agent": "a1", "text": "Run the tests.", "refs": []string{"comment 999"}}, // an item not read
		{"agent": "a1", "text": "Run the tests.", "merges": []int64{other}},        // another agent's lesson merged
		{"agent": "a1", "text": "Run the tests.", "merges": []int64{mine, mine}},   // one merged twice
		{"agent": "a1", "text": "Run\nthe tests."},                                 // two lines
	} {
		c := comment(t, d, tid, person, "You skipped the tests again", int64(100+i))
		if _, ok := l["refs"]; !ok {
			l["refs"] = []string{"comment " + itoa(c)}
		}
		a := &answer{rules: []map[string]any{rule(0, "Run the tests.", 1, "skipped the tests", "comment "+itoa(c))}, lessons: []map[string]any{l}}
		res, err := Step(context.Background(), d, person, a.model, true)
		if err != nil || !res.Written || len(res.Lessons) != 0 || len(res.Skipped) != 1 || res.Skipped[0].Why == "" {
			t.Fatalf("%d: %+v %v", i, res, err)
		}
		if ls := lessonsOf(t, d, "a1"); len(ls) != 1 || ls[0].ID != mine {
			t.Fatalf("%d: %+v", i, ls)
		}
		_, _ = d.Exec("DELETE FROM lessons WHERE agent = ?", person)
	}
}

// An agent's lessons stay within its cap: a lesson that would pass it is left out while the rest is written, and a merge that replaces
// lessons saying the same thing makes room.
func TestAnAgentLessonStaysWithinItsCap(t *testing.T) {
	d := board(t)
	long := strings.Repeat("z", 280)
	var ids []int64
	for i := 0; db.Fits("a1", append(lessonsOf(t, d, "a1"), db.Lesson{Text: long + "x"})); i++ {
		id, err := db.AddLesson(d, "a1", long+itoa(int64(i)), 0, "a1")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "You merged without running the checks", 110)
	text := "Run the checks before you merge: " + strings.Repeat("y", 240)
	a := &answer{lessons: []map[string]any{{"agent": "a1", "text": text, "refs": []string{"comment " + itoa(c)}}}}
	res, err := Step(context.Background(), d, person, a.model, true)
	if err != nil || !res.Written || len(res.Skipped) != 1 || len(lessonsOf(t, d, "a1")) != len(ids) {
		t.Fatal("a lesson past the cap must be left out, the step written", res, err)
	}
	c2 := comment(t, d, tid, person, "Again you merged without the checks", 120)
	a.lessons = []map[string]any{{"agent": "a1", "text": text, "refs": []string{"comment " + itoa(c2)}, "merges": ids[:2]}}
	if res, err := Step(context.Background(), d, person, a.model, true); err != nil || len(res.Skipped) != 0 {
		t.Fatal(res, err)
	}
	ls := lessonsOf(t, d, "a1")
	if len(ls) != len(ids)-1 || ls[len(ls)-1].Text != text || ls[len(ls)-1].Task != tid || !db.Fits("a1", ls) {
		t.Fatalf("%+v", ls)
	}
}

func lessonsOf(t *testing.T, q db.Q, agent string) []db.Lesson {
	t.Helper()
	ls, err := db.LiveLessons(q, agent)
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

// Past MaxRules the fold's least said rules go, though the bytes would fit; a person's never.
func TestPastMaxRulesTheLeastSaidGo(t *testing.T) {
	d := board(t)
	hand, _ := db.AddLesson(d, person, "by hand", 0, person)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "This is a rule", 100)
	var rules []map[string]any
	for i := range MaxRules + 5 {
		rules = append(rules, rule(0, "rule "+itoa(int64(i)), 1, "This is a rule", "comment "+itoa(c)))
	}
	a := &answer{rules: rules}
	if _, err := Step(context.Background(), d, person, a.model, true); err != nil {
		t.Fatal(err)
	}
	if ls := live(t, d); len(ls) != MaxRules || ls[0].ID != hand {
		t.Fatalf("%d rules kept", len(ls))
	}
}

// A kept rule said again but returned with no id, in its own words, is that rule: it counts up and keeps its quote, no copy.
func TestAKeptRuleReturnedWithoutItsIDIsThatRule(t *testing.T) {
	d := board(t)
	twice, _ := db.AddLesson(d, person, `Fix the cause. (said 2 times; "근본수정", comment 5)`, 0, By)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Fix the cause, again", 100)
	r := rule(0, "Fix the cause.", 1, "", "")
	r["refs"] = []string{"comment " + itoa(c)}
	a := &answer{rules: []map[string]any{r}}
	if _, err := Step(context.Background(), d, person, a.model, true); err != nil {
		t.Fatal(err)
	}
	if ls := live(t, d); len(ls) != 1 || ls[0].Text != `Fix the cause. (said 3 times; "근본수정", comment 5)` || ls[0].ID == twice {
		t.Fatalf("%+v", ls)
	}
}
