package taskwrites

import (
	"fmt"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
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
	n := 0
	as := func(agent, method, params string) map[string]any {
		t.Helper()
		n++
		s := rpc.NewServer(auth{agent}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		RegisterLessons(s)
		return call(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":%q,"params":{%s"request_id":"r%d"}}`, method, params, n))[0]
	}
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
			if !strings.Contains(got, "hl lesson merge") {
				t.Fatalf("a full set: %q", got)
			}
			break
		}
		if i > 10 {
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
