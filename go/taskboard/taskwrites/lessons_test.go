package taskwrites

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

// An agent keeps and merges its own lessons, a full set is refused with the way to merge, anyone lists them, and only the founder removes
// one.
func TestAnAgentKeepsItsLessonsAndTheFounderRemovesOne(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker", "gp.other")
	worker := teamtest.Key(t, home, "gp.worker")
	exec(t, d, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'story part', %q, 'gp', 1), (2, 'not mine', 'gp.other', 'gp', 1)", worker))
	as := lessonCaller(t, d)
	id := func(r map[string]any) float64 {
		t.Helper()
		res, _ := r["result"].(map[string]any)
		if res == nil {
			t.Fatalf("%v", r)
		}
		return res["id"].(float64)
	}
	one := id(as(worker, "lesson.add", `"text":"say what is left out","task":1,`))
	two := id(as(worker, "lesson.add", `"text":"say what was left out","task":1,`))
	if got := errText(as(worker, "lesson.add", `"text":"x","task":99,`)); !strings.Contains(got, "no task T99") {
		t.Errorf("a lesson on no task: %q", got)
	}
	if got := errText(as(worker, "lesson.add", `"text":"x","task":2,`)); !strings.Contains(got, "not your task") {
		t.Errorf("a lesson on another's task: %q", got)
	}
	if got := errText(as(worker, "lesson.add", `"text":"two\nlines","task":1,`)); !strings.Contains(got, "one line") {
		t.Errorf("a lesson over two lines: %q", got)
	}
	long := strings.Repeat("y", 480)
	for i := 0; ; i++ {
		r := as(worker, "lesson.add", fmt.Sprintf(`"text":%q,"task":1,`, long))
		if got := errText(r); got != "" {
			if !strings.Contains(got, "hl lesson merge") || strings.Contains(got, "--said") {
				t.Fatalf("a full set: %q", got)
			}
			break
		}
		if i > 40 {
			t.Fatal("never full")
		}
	}
	if got := errText(as(teamtest.Key(t, home, "gp.other"), "lesson.merge", fmt.Sprintf(`"ids":[%v,%v],"text":"x",`, one, two))); !strings.Contains(got, "not one of your live lessons") {
		t.Errorf("another agent's merge: %q", got)
	}
	if got := errText(as(worker, "lesson.merge", fmt.Sprintf(`"ids":[%v,%v],"text":"x",`, one, one))); !strings.Contains(got, "two or more different") {
		t.Errorf("a merge of one lesson twice: %q", got)
	}
	merged := id(as(worker, "lesson.merge", fmt.Sprintf(`"ids":[%v,%v],"text":"say what is left out, and why",`, one, two)))
	if got := errText(as(worker, "lesson.remove", fmt.Sprintf(`"id":%v,`, merged))); got == "" {
		t.Error("an agent removed a lesson")
	}
	if r := as("", "lesson.remove", fmt.Sprintf(`"id":%v,`, merged)); errText(r) != "" {
		t.Fatalf("the founder's remove: %v", r)
	}
	list := as("", "lessons.list", fmt.Sprintf(`"agent":%q,`, worker))["result"].(map[string]any)
	for _, it := range list["items"].([]any) {
		if text := it.(map[string]any)["text"]; text != long {
			t.Errorf("listed %v after its merge and removal", text)
		}
	}
}

// A lesson about the founder cites their own words (a comment or a message of theirs), is kept under their key by whichever agent adds
// it, any agent may merge them, and their own cap is apart from each agent's.
func TestALessonAboutTheFounderCitesTheirWordsAndHasItsOwnCap(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker", "gp.other")
	worker, other := teamtest.Key(t, home, "gp.worker"), teamtest.Key(t, home, "gp.other")
	founder, err := addr.OwnerOf(d, worker)
	if err != nil {
		t.Fatal(err)
	}
	exec(t, d, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'story part', %q, 'gp', 1)", worker))
	exec(t, d, fmt.Sprintf("INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(7, 1, %q, 'in Korean, please', 1), (8, 1, %q, 'noted', 2)",
		founder, worker))
	exec(t, d, fmt.Sprintf("INSERT INTO requests(id, ts, requester, target, body, state, kind, ref) VALUES(5, 1, %q, %q, 'short, please', 'landed', 'say', 'abcdef123456')",
		founder, worker))
	as := lessonCaller(t, d)
	if got := errText(as(worker, "lesson.add", `"text":"write in Korean","task":1,"about":"founder",`)); !strings.Contains(got, "said names the founder's own words") {
		t.Errorf("no citation: %q", got)
	}
	if got := errText(as(worker, "lesson.add", `"text":"write in Korean","task":1,"about":"founder","said":8,`)); !strings.Contains(got, "comment 8 is not the founder's") {
		t.Errorf("an agent's comment cited: %q", got)
	}
	for _, said := range []string{`7`, `"msg_abcdef1"`} {
		if r := as(worker, "lesson.add", `"text":"write in Korean","task":1,"about":"founder","said":`+said+`,`); errText(r) != "" {
			t.Fatalf("said %s: %v", said, r)
		}
	}
	list := func(whose string) []any {
		t.Helper()
		return as(worker, "lessons.list", fmt.Sprintf(`"agent":%q,`, whose))["result"].(map[string]any)["items"].([]any)
	}
	items := list(founder)
	if len(items) != 2 || items[0].(map[string]any)["text"] != "write in Korean (the founder, comment 7)" ||
		items[1].(map[string]any)["text"] != "write in Korean (the founder, msg_abcdef1)" || items[0].(map[string]any)["created_by"] != worker {
		t.Fatalf("the founder's lessons: %v", items)
	}
	if len(list(worker)) != 0 {
		t.Error("a lesson about the founder is kept as the agent's own")
	}
	ids := fmt.Sprintf(`[%v,%v]`, items[0].(map[string]any)["id"], items[1].(map[string]any)["id"])
	if r := as(other, "lesson.merge", `"ids":`+ids+`,"text":"write in Korean","about":"founder","said":7,`); errText(r) != "" {
		t.Fatalf("another agent's merge of the founder's: %v", r)
	}
	if items := list(founder); len(items) != 1 || items[0].(map[string]any)["created_by"] != other {
		t.Fatalf("after the merge: %v", items)
	}
	long := strings.Repeat("y", 460)
	for i := 0; ; i++ {
		got := errText(as(worker, "lesson.add", fmt.Sprintf(`"text":%q,"task":1,"about":"founder","said":7,`, long)))
		if got != "" {
			if !strings.Contains(got, "--about founder --said") {
				t.Fatalf("the founder's full set: %q", got)
			}
			break
		}
		if i > 40 {
			t.Fatal("never full")
		}
	}
	if got := errText(as(worker, "lesson.add", `"text":"say what is left out","task":1,`)); got != "" {
		t.Errorf("the founder's full set stopped the agent's own: %q", got)
	}
}

// lessonCaller calls a lessons method on d as agent ("" for the founder), each call with a request id of its own.
func lessonCaller(t *testing.T, d *sql.DB) func(agent, method, params string) map[string]any {
	n := 0
	return func(agent, method, params string) map[string]any {
		t.Helper()
		n++
		s := rpc.NewServer(auth{agent}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		RegisterLessons(s)
		return call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":%q,"params":{%s"request_id":"r%d"}}`, method, params, n))[0]
	}
}

// A person's rule lists as its area, the rule alone, how often it was said and the quote kept, with the comment or chat message it was
// said in; an agent's own lesson is said once and has neither.
func TestAPersonsRuleListsItsAreaCountAndQuote(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker")
	worker := teamtest.Key(t, home, "gp.worker")
	founder, err := addr.OwnerOf(d, worker)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`Screen: Use flat rows. (said 18 times; "No cards.", comment 7)`, `Work: Subtract. (said 3 times; "Keep it simple.", msg_abcdef1)`,
		"Work: by hand"} {
		if _, err := db.AddLesson(d, founder, text, 4, "rules_fold"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.AddLesson(d, worker, "Run the tests first.", 0, worker); err != nil {
		t.Fatal(err)
	}
	as := lessonCaller(t, d)
	list := func(whose string) string {
		t.Helper()
		b, _ := json.Marshal(as(worker, "lessons.list", fmt.Sprintf(`"agent":%q,`, whose))["result"].(map[string]any)["items"])
		return regexp.MustCompile(`"created_at":"[^"]*",`).ReplaceAllString(string(b), "")
	}
	want := `[{"agent":"` + founder + `","area":"screen","count":18,"created_by":"rules_fold","id":1,"quote":{"comment":7,"task":4,"text":"No cards."},"task":4,"text":"Use flat rows."},` +
		`{"agent":"` + founder + `","area":"work","count":3,"created_by":"rules_fold","id":2,"quote":{"message":"msg_abcdef1","task":4,"text":"Keep it simple."},"task":4,"text":"Subtract."},` +
		`{"agent":"` + founder + `","area":"work","count":1,"created_by":"rules_fold","id":3,"task":4,"text":"by hand"}]`
	if got := list(founder); got != want {
		t.Errorf("the founder's rules:\n got %s\nwant %s", got, want)
	}
	if got := list(worker); got != `[{"agent":"`+worker+`","count":1,"created_by":"`+worker+`","id":4,"text":"Run the tests first."}]` {
		t.Errorf("an agent's own lesson: %s", got)
	}
}
