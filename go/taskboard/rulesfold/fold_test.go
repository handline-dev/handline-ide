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
		rule(0, "screen", "Keep screen copy short.", 1, "Too many words on this screen", "comment "+itoa(c2)),
		rule(0, "work", "Say what changed before asking for a look.", 1, "Always tell me what changed", "msg_"+ref(t, d, m1)),
	}}
	res, err := Step(context.Background(), d, person, a.model, nil, true)
	if err != nil || !res.Written {
		t.Fatal(res, err)
	}
	if len(a.asked) != 1 || strings.Contains(a.asked[0], `"said":"OK"`) || !strings.Contains(a.asked[0], "Here is the settings tab.") ||
		!strings.Contains(a.asked[0], "Done, merged.") {
		t.Fatalf("prompt: the ack must be left out and each item carry what it answered:\n%s", a.asked[0])
	}
	got := live(t, d)
	if len(got) != 2 || got[0].Text != `Screen: Keep screen copy short. (said once; "Too many words on this screen", comment `+itoa(c2)+")" ||
		got[0].Task != tid || !strings.HasPrefix(got[1].Text, "Work: Say what changed") || got[1].Task != 0 || got[0].CreatedBy != By {
		t.Fatalf("lessons: %+v", got)
	}
	if cur, _ := ReadCursors(d, person); cur.Comments != c2 || cur.Chat != m1 || c1 >= c2 {
		t.Fatalf("cursors %+v, want comments %d chat %d", cur, c2, m1)
	}
	a.asked = nil
	if res, err := Step(context.Background(), d, person, a.model, nil, true); err != nil || len(a.asked) != 0 || res.Written {
		t.Fatal("a step with nothing new must ask nothing", res, err)
	}
}

// A dry run asks and validates but writes nothing and moves no cursor.
func TestADryRunWritesNothing(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Never show internal ids on screen", 100)
	a := &answer{rules: []map[string]any{rule(0, "screen", "Show names, not ids.", 1, "Never show internal ids", "comment "+itoa(c))}}
	res, err := Step(context.Background(), d, person, a.model, nil, false)
	if err != nil || res.Written || len(res.Rules) != 1 {
		t.Fatal(res, err)
	}
	if cur, _ := ReadCursors(d, person); cur != (Cursors{}) || len(live(t, d)) != 0 {
		t.Fatal("a dry run wrote", cur)
	}
}

// Nothing done before is lost: an answer that leaves out a rule a person wrote, or one said more than once, or lowers a count, or quotes
// what was not read, is refused, and the cursors stay. An id the model made up is a new rule.
func TestAnAnswerThatLosesARuleIsRefusedAndTheCursorsStay(t *testing.T) {
	d := board(t)
	hand, _ := db.AddLesson(d, person, "Write to the reviewer in Korean.", 0, person)
	twice, _ := db.AddLesson(d, person, `Work: Fix the cause. (said 2 times; "근본수정", comment 5)`, 0, By)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Do not use cards for small things", 100)
	keepBoth := []map[string]any{rule(hand, "work", "Write to the reviewer in Korean.", 1, "", ""),
		rule(twice, "work", "Fix the cause.", 2, "근본수정", "comment 5")}
	for name, rules := range map[string][]map[string]any{
		"a person's rule left out":   {keepBoth[1]},
		"a rule said twice left out": {keepBoth[0]},
		"a count lowered":            {keepBoth[0], rule(twice, "work", "Fix the cause.", 1, "근본수정", "comment 5")},
		"a quote not read":           append(keepBoth, rule(0, "screen", "No cards.", 1, "Do not use cards", "comment 12345")),
		"a rule twice":               append(keepBoth, rule(0, "screen", "No cards.", 1, "Do not use cards", "comment "+itoa(c), hand)),
		"a bad area":                 append(keepBoth, rule(0, "looks", "No cards.", 1, "Do not use cards", "comment "+itoa(c))),
	} {
		a := &answer{rules: rules}
		if _, err := Step(context.Background(), d, person, a.model, nil, true); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if cur, _ := ReadCursors(d, person); cur != (Cursors{}) || len(live(t, d)) != 2 {
			t.Fatalf("%s: wrote something", name)
		}
	}
	a := &answer{rules: append(keepBoth, rule(99, "screen", "No cards for small things.", 1, "Do not use cards", "comment "+itoa(c), 98))}
	if res, err := Step(context.Background(), d, person, a.model, nil, true); err != nil || !res.Written || len(live(t, d)) != 3 {
		t.Fatal(res, err)
	}
	// The person's own rule is kept as it was: an unchanged rule is not rewritten.
	if ls := live(t, d); ls[0].ID != hand || ls[1].ID != twice {
		t.Fatalf("kept rules rewritten: %+v", ls)
	}
}

// A rule said again counts up and keeps its id's history; kept rules the model merges become one, and a rule the fold wrote said once
// may go; a rule merged from a person's keeps that protection.
func TestRulesCountUpMergeAndOnlyTheFoldsOnceSaidMayGo(t *testing.T) {
	d := board(t)
	hand, _ := db.AddLesson(d, person, "Lists are flat rows.", 0, person)
	once, _ := db.AddLesson(d, person, `Screen: No cards. (said once; "카드 쓰지 말자", comment 3)`, 0, By)
	gone, _ := db.AddLesson(d, person, `Work: Merge quietly. (said once; "조용히", comment 4)`, 0, By)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Again cards everywhere, use flat rows", 100)
	a := &answer{rules: []map[string]any{rule(hand, "screen", "Lists are flat rows, never cards.", 3, "use flat rows", "comment "+itoa(c), once)}}
	res, err := Step(context.Background(), d, person, a.model, func(int64) string { return "screen" }, true)
	if err != nil || len(res.Dropped) != 1 || res.Dropped[0].ID != gone {
		t.Fatal(res, err)
	}
	ls := live(t, d)
	if len(ls) != 1 || ls[0].Text != `Screen: Lists are flat rows, never cards. (said 3 times; "use flat rows", comment `+itoa(c)+")" || ls[0].Task != tid {
		t.Fatalf("%+v", ls)
	}
	var mergedInto int64
	_ = d.QueryRow("SELECT merged_into FROM lessons WHERE id = ?", hand).Scan(&mergedInto)
	if mergedInto != ls[0].ID {
		t.Fatal("the person's rule must be kept, merged into the new one")
	}
	// Merged from a person's rule, it stays protected though the fold wrote it.
	k, _ := keptRules(d, person)
	if !k[0].keep {
		t.Fatal("a rule merged from a person's must keep its protection")
	}
}

// An item on a task of known kind of work gives its quote's rule that area, whatever the model said.
func TestTheQuotesTaskKindIsTheRulesArea(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Report the cause, not the symptom", 100)
	a := &answer{rules: []map[string]any{rule(0, "work", "Report the cause.", 1, "Report the cause", "comment "+itoa(c))}}
	if _, err := Step(context.Background(), d, person, a.model, func(int64) string { return db.AreaScreen }, true); err != nil {
		t.Fatal(err)
	}
	if ls := live(t, d); !strings.HasPrefix(ls[0].Text, "Screen: ") {
		t.Fatal(ls[0].Text)
	}
}

// A lesson added while the model answered (a Redo) leaves the step unwritten, its batch read again next time.
func TestARuleAddedWhileTheModelAnsweredLeavesTheStepUnwritten(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "Show before and after pictures", 100)
	a := &answer{rules: []map[string]any{rule(0, "work", "Show pictures.", 1, "Show before and after", "comment "+itoa(c))},
		during: func() { _, _ = db.AddLesson(d, person, "from a Redo", 0, person) }}
	if _, err := Step(context.Background(), d, person, a.model, nil, true); !errors.Is(err, ErrMoved) {
		t.Fatal(err)
	}
	if cur, _ := ReadCursors(d, person); cur != (Cursors{}) || len(live(t, d)) != 1 {
		t.Fatal("wrote past a change", cur)
	}
}

// Past an area's cap, the fold's least said rules go until it fits; a rule said twice outlasts those said once. A count rises only by the items read that say it. A set
// that cannot fit is refused.
func TestPastTheCapTheLeastSaidDroppableRulesGo(t *testing.T) {
	d := board(t)
	tid := task(t, d, "x")
	c := comment(t, d, tid, person, "This is a rule about screens", 100)
	c2 := comment(t, d, tid, person, "Again: a rule about screens", 110)
	long := strings.Repeat("x", 110)
	twice := rule(0, "screen", "Said twice.", 2, "This is a rule", "comment "+itoa(c))
	twice["refs"] = []string{"comment " + itoa(c), "comment " + itoa(c2)}
	rules := []map[string]any{rule(0, "screen", "Claimed often.", 9, "This is a rule", "comment "+itoa(c))}
	for i := range 90 {
		rules = append(rules, rule(0, "screen", long+itoa(int64(i)), 1, "This is a rule", "comment "+itoa(c)))
	}
	a := &answer{rules: append(rules, twice)}
	res, err := Step(context.Background(), d, person, a.model, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	ls := live(t, d)
	if len(ls) < 2 || len(ls) >= 92 || !db.Fits(person, ls) {
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
	for i := range 90 {
		_, _ = d2.Exec("INSERT INTO lessons(agent, text, created_at, created_by) VALUES(?, ?, 1, ?)", person, "Screen: "+long+itoa(int64(i)), person)
	}
	tid2 := task(t, d2, "x")
	c3 := comment(t, d2, tid2, person, "one more screen rule", 100)
	k, _ := keptRules(d2, person)
	var all []map[string]any
	for _, r := range k {
		all = append(all, rule(r.ID, "screen", r.Text, 1, "", ""))
	}
	a2 := &answer{rules: append(all, rule(0, "screen", "x", 1, "one more", "comment "+itoa(c3)))}
	if _, err := Step(context.Background(), d2, person, a2.model, nil, true); err == nil {
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
				return []byte(`{"rules":[{"id":0,"area":"work","text":"Keep the standard.","count":1,"quote":"a lasting standard","quote_ref":"` + r + `","refs":[]}]}`), nil
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
	a := &answer{rules: []map[string]any{rule(hand, "work", text, 1, "", ""), rule(0, "work", "Show pictures.", 1, "Show before and after", "comment "+itoa(c))}}
	if _, err := Step(context.Background(), d, person, a.model, nil, true); err != nil {
		t.Fatal(err)
	}
	if ls := live(t, d); ls[0].ID != hand || ls[0].Text != text {
		t.Fatalf("%+v", ls[0])
	}
	// said again, it counts up and keeps the person's words, however long
	c2 := comment(t, d, tid, person, "Ask me one item at a time, again", 200)
	pics := live(t, d)[1].ID
	a.rules = []map[string]any{rule(hand, "work", text, 2, "Ask me one item at a time", "comment "+itoa(c2)),
		rule(pics, "work", "Show pictures.", 1, "Show before and after", "comment "+itoa(c))}
	if _, err := Step(context.Background(), d, person, a.model, nil, true); err != nil {
		t.Fatal("a person's long rule said again must not stop the fold: ", err)
	}
	if ls := live(t, d); len(ls) != 2 || !strings.HasPrefix(ls[1].Text, "Work: "+text) || !strings.Contains(ls[1].Text, "said 2 times") {
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
		if res, err := Step(context.Background(), d, person, a.model, nil, true); err != nil || !res.Written {
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
