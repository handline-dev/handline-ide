package reads

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/protocol/rpc"
)

type searchHit struct {
	Kind, At, Snippet, Field, Title, Agent, Scope, From string
	Task, Memo, Request                                 int64
}

func searchOf(t *testing.T, e *Env, params map[string]any) (hits []searchHit, more bool) {
	t.Helper()
	got, err := e.search(nil, params)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var out struct {
		Hits []searchHit
		More bool
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.Hits, out.More
}

// line is a hit as the test names it: its kind, what opens it and where the text was.
func (h searchHit) line() string {
	switch h.Kind {
	case "task":
		return fmt.Sprintf("task %d %s", h.Task, h.Field)
	case "comment":
		return fmt.Sprintf("comment %d on %d by %s", h.Memo, h.Task, h.From)
	}
	return fmt.Sprintf("message %d in %s %s task %d", h.Request, h.Agent, h.Scope, h.Task)
}

func TestSearchFindsTasksCommentsAndTheMessagesAChatDraws(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	addAgent(t, e, "cy", "")
	bob, cy := teamtest.Key(t, e.Home, "bob"), teamtest.Key(t, e.Home, "cy")
	for _, q := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'Rename the Checkout screen', '" + bob + "', 'p1', 100)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(2, 'Second', '" + bob + "', 'p1', 200)",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, done_evidence) VALUES(3, 'Third', '" + bob + "', 'p1', 300, 'abc1234 checkout landed')",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at, report) VALUES(4, 'Fourth', '" + bob + "', 'p1', 400, 'the checkout holds')",
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(5, 'Nothing here', '" + bob + "', 'p1', 500)",
		// task 6 was opened by bob, and the first request linked to it is the person's: not its brief (supervisor.briefOf), so a message
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(6, 'Sixth', '" + bob + "', '" + bob + "', 600)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(19, 219, 'p1', '" + bob + "', 'checkout, asked with a task of yours', 'landed', 'say')",
		"INSERT INTO task_requests(task_id, request_id) VALUES(6, 19)",
		// task 2's brief, and a later delivery of the same task
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(10, 210, 'p1', '" + bob + "', 'Fix the\ncheckout flow', 'landed', 'task')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(24, 240, '" + bob + "', '" + cy + "', 'checkout again, to you', 'landed', 'task')",
		"INSERT INTO task_requests(task_id, request_id) VALUES(2, 10), (2, 24)",
		"INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(5, 1, '" + bob + "', 'I agree on the CHECKOUT wording', 150)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(20, 220, 'p1', '" + bob + "', 'a checkout question', 'landed', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(21, 221, '" + bob + "', '" + cy + "', 'checkout between agents', 'landed', 'say')",
		// none of these is drawn by a chat: taken back, a comment's forward, a notice a digest listed, a message between people, the
		// daemon's own notice
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(22, 222, 'p1', '" + bob + "', 'checkout taken back', 'withdrawn', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(23, 223, '" + bob + "', '" + cy + "', 'checkout comment, forwarded', 'landed', 'task')",
		"INSERT INTO memo_tells(memo_id, reader, request_id) VALUES(5, '" + cy + "', 23)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind, notice, delivered_at) VALUES(25, 225, '" + bob + "', '" + cy + "', 'checkout notice', 'landed', 'say', 1, 226)",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(26, 226, 'p1', 'p1', 'checkout to myself', 'landed', 'say')",
		"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(27, 227, 'sys', '" + bob + "', 'checkout nudge', 'landed', 'say')",
	} {
		run(t, e, q)
	}
	hits, more := searchOf(t, e, map[string]any{"query": " checkout "})
	var got []string
	for _, h := range hits {
		got = append(got, h.line())
	}
	want := []string{
		"task 4 result", "task 3 result", "task 2 brief", "task 1 title",
		"comment 5 on 1 by " + bob,
		"message 24 in " + cy + " all task 2", "message 21 in " + cy + " all task 0", "message 20 in " + bob + " mine task 0",
		"message 19 in " + bob + " mine task 6",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || more {
		t.Fatalf("more %v, hits:\n%s\nwant:\n%s", more, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	by := map[string]searchHit{}
	for _, h := range hits {
		by[h.line()] = h
	}
	for line, w := range map[string]searchHit{
		"task 2 brief":                          {Snippet: "Fix the checkout flow", Title: "Second", At: "1970-01-01T00:03:20Z"},
		"task 1 title":                          {Snippet: "Rename the Checkout screen", Title: "Rename the Checkout screen", At: "1970-01-01T00:01:40Z"},
		"task 4 result":                         {Snippet: "the checkout holds", Title: "Fourth", At: "1970-01-01T00:06:40Z"},
		"comment 5 on 1 by " + bob:              {Snippet: "I agree on the CHECKOUT wording", Title: "Rename the Checkout screen", At: "1970-01-01T00:02:30Z"},
		"message 24 in " + cy + " all task 2":   {Snippet: "checkout again, to you", Title: "Second", At: "1970-01-01T00:04:00Z"},
		"message 20 in " + bob + " mine task 0": {Snippet: "a checkout question", At: "1970-01-01T00:03:40Z"},
	} {
		h := by[line]
		if h.Snippet != w.Snippet || h.Title != w.Title || h.At != w.At {
			t.Errorf("%s: snippet %q title %q at %s, want %q %q %s", line, h.Snippet, h.Title, h.At, w.Snippet, w.Title, w.At)
		}
	}
	// What no chat draws does not count toward the limit: the newest three messages are all three of a limit of three.
	limited, _ := searchOf(t, e, map[string]any{"query": "checkout", "limit": json.Number("3")})
	if n := len(limited); n != 7 || limited[6].Request != 20 {
		t.Errorf("a limit of 3 gives %d hits, the last %v", n, limited[n-1])
	}
	if h := by["message 20 in "+bob+" mine task 0"]; h.From != "p1" {
		t.Errorf("a message hit says who sent it: %q", h.From)
	}
}

func TestSearchTakesPercentAndUnderscoreAsPlainCharacters(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for i, title := range []string{"100% done", "100 done", "a_b", "axb", `back\slash`} {
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(?, ?, 'a1', 'p1', 1)", i+1, title)
	}
	for query, want := range map[string]string{"100%": "100% done", "a_b": "a_b", `k\s`: `back\slash`, "%": "100% done", "_": "a_b"} {
		hits, _ := searchOf(t, e, map[string]any{"query": query})
		if len(hits) != 1 || hits[0].Title != want {
			t.Errorf("%q finds %v, want only %q", query, hits, want)
		}
	}
}

func TestSearchBoundsItsQueryAndItsHits(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	for i := 1; i <= 3; i++ {
		run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(?, '가나다 plan', 'a1', 'p1', 1)", i)
		run(t, e, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(?, 1, 'a1', 'the plan', 1)", i)
	}
	for _, bad := range []struct {
		params map[string]any
		field  string
	}{
		{map[string]any{}, "query"},
		{map[string]any{"query": "  "}, "query"},
		{map[string]any{"query": 7}, "query"},
		{map[string]any{"query": strings.Repeat("가", 201)}, "query"},
		{map[string]any{"query": "plan", "limit": json.Number("0")}, "limit"},
		{map[string]any{"query": "plan", "limit": json.Number("51")}, "limit"},
		{map[string]any{"query": "plan", "limit": json.Number("1.5")}, "limit"},
	} {
		_, err := e.search(nil, bad.params)
		var re *rpc.Error
		if !errors.As(err, &re) || re.Code != "bad_request" || re.Data["field"] != bad.field {
			t.Errorf("%v: %v, want bad_request naming %s", bad.params, err, bad.field)
		}
	}
	// A query of exactly the most characters is read, counted in characters and not bytes.
	if hits, _ := searchOf(t, e, map[string]any{"query": strings.Repeat("가", 200)}); len(hits) != 0 {
		t.Errorf("%v", hits)
	}
	hits, more := searchOf(t, e, map[string]any{"query": "PLAN", "limit": json.Number("2")})
	var got []string
	for _, h := range hits {
		got = append(got, h.line())
	}
	if want := "task 3 title, task 2 title, comment 3 on 1 by a1, comment 2 on 1 by a1"; strings.Join(got, ", ") != want || !more {
		t.Errorf("more %v: %s, want %s", more, strings.Join(got, ", "), want)
	}
	if hits, more := searchOf(t, e, map[string]any{"query": "가나다", "limit": json.Number("3")}); len(hits) != 3 || more {
		t.Errorf("the limit itself is not more: %d hits, more %v", len(hits), more)
	}
}

func TestASnippetIsOneLineAroundTheMatchWithItsCutsMarked(t *testing.T) {
	t.Parallel()
	pad := strings.Repeat("가나다라 ", 60) // 300 characters
	for _, c := range []struct {
		name, text, query string
		check             func(s string) bool
	}{
		{"short text is whole, a tab as spaces", "a  Needle\there", "needle", func(s string) bool { return s == "a  Needle   here" }},
		{"line breaks are spaces", "one\r\ntwo needle", "needle", func(s string) bool { return s == "one two needle" }},
		{"a match at the start keeps the start", "needle " + pad, "needle", func(s string) bool {
			return strings.HasPrefix(s, "needle 가나다라") && strings.HasSuffix(s, "…")
		}},
		{"a match in the middle is cut at both ends", pad + "the needle is here " + pad, "NEEDLE", func(s string) bool {
			return strings.HasPrefix(s, "…") && strings.HasSuffix(s, "…") && strings.Contains(s, "the needle is here")
		}},
		{"a match near the end keeps the end", pad + "the needle", "needle", func(s string) bool {
			return strings.HasPrefix(s, "…") && strings.HasSuffix(s, "the needle")
		}},
		{"text that does not hold it on one line shows its start", "nee\ndle " + pad, "nee\ndle", func(s string) bool {
			return strings.HasPrefix(s, "nee dle 가나다라")
		}},
		{"other letters match only themselves", pad + "Ünder " + pad, "ünder", func(s string) bool { return strings.HasPrefix(s, "가나다라") }},
	} {
		s := snippetOf(c.text, c.query)
		if n := utf8.RuneCountInString(s); n > snippetMax || strings.ContainsAny(s, "\n\t") || !c.check(s) {
			t.Errorf("%s: %d characters: %q", c.name, n, s)
		}
	}
}
