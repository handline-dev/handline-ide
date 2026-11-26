package reads

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// search is a person's one box over what was agreed: tasks (title, brief, what the end kept), comments and the messages a chat draws.
// It reads with LIKE: a scan of every message of a long-used install takes tens of milliseconds, so there is no index to keep.

const (
	searchQueryMax    = 200 // characters of a query
	searchHitsDefault = 10  // hits of each kind
	searchHitsMax     = 50
	snippetMax        = 160 // characters of a snippet, its … included
	snippetLead       = 40  // characters kept before the match
)

// likeOf is text as a LIKE pattern that finds it anywhere, its % and _ plain characters; the query says ESCAPE '\'.
func likeOf(text string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(text) + "%"
}

// foldASCII lowers A to Z and nothing else, which is how LIKE compares: any other letter matches only itself.
func foldASCII(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, s)
}

// holds is whether LIKE finds query in text.
func holds(text, query string) bool { return strings.Contains(foldASCII(text), foldASCII(query)) }

// snippetOf is one line of text around the first place it holds query, at most snippetMax characters, with … where it was cut; the
// start of the text when the line does not hold it (a match across a line break).
func snippetOf(text, query string) string {
	line := textsafe.Strip(textsafe.Oneline(text))
	at := 0
	if i := strings.Index(foldASCII(line), foldASCII(query)); i > 0 {
		at = utf8.RuneCountInString(line[:i])
	}
	rs := []rune(line)
	if len(rs) <= snippetMax {
		return line
	}
	from := max(0, at-snippetLead)
	if len(rs)-from <= snippetMax-1 {
		// The match is near the end: the snippet ends with the text and reaches back as far as it may.
		return "…" + strings.TrimSpace(string(rs[len(rs)-(snippetMax-1):]))
	}
	room := snippetMax - 1
	if from > 0 {
		room--
	}
	out := strings.TrimSpace(string(rs[from:from+room])) + "…"
	if from > 0 {
		out = "…" + out
	}
	return out
}

const searchTasksSQL = "SELECT id, title, created_at, brief, done_evidence, report FROM (SELECT t.id, t.title, t.created_at, t.done_evidence, t.report, " +
	"(SELECT r.body FROM task_requests l JOIN requests r ON r.id = l.request_id WHERE l.task_id = t.id ORDER BY l.rowid LIMIT 1) AS brief FROM tasks t) " +
	`WHERE title LIKE ? ESCAPE '\' OR brief LIKE ? ESCAPE '\' OR done_evidence LIKE ? ESCAPE '\' OR report LIKE ? ESCAPE '\' ORDER BY id DESC LIMIT ?`

const searchCommentsSQL = "SELECT m.id, m.task_id, m.author, m.body, m.created_at, t.title FROM task_memos m JOIN tasks t ON t.id = m.task_id " +
	`WHERE m.body LIKE ? ESCAPE '\' ORDER BY m.id DESC LIMIT ?`

// searchMessagesSQL is the requests a chat draws that hold the text: not one taken back or never delivered, not a notice a digest
// listed or a comment's forward (chatScope leaves both out), not the daemon's own notices (no screen shows them as talk), and not a
// task's brief, which is that task's hit. A message between people is in no agent's chat. All of it is said here, before the limit, so
// the limit counts hits.
var searchMessagesSQL = "SELECT id, ts, requester, target, " +
	"(SELECT MIN(task_id) FROM task_requests WHERE request_id = requests.id) AS task_id, " +
	"(SELECT t.title FROM tasks t JOIN task_requests q ON q.task_id = t.id WHERE q.request_id = requests.id ORDER BY t.id LIMIT 1) AS task_title, " +
	"body FROM requests WHERE state NOT IN " + neverReached + ` AND body LIKE ? ESCAPE '\'` +
	" AND requester != '" + actor.Sys + "' AND NOT (requester IN " + personKeys + " AND target IN " + personKeys + ")" +
	" AND NOT " + db.Digested("") + " AND id NOT IN (" + db.ForwardedSQL + ")" +
	" AND NOT EXISTS (SELECT 1 FROM task_requests l WHERE l.request_id = requests.id AND l.rowid = " +
	"(SELECT MIN(f.rowid) FROM task_requests f WHERE f.task_id = l.task_id)) ORDER BY id DESC LIMIT ?"

func (e *Env) search(_ *rpc.Conn, params map[string]any) (any, error) {
	text, ok := params["query"].(string)
	text = strings.TrimSpace(text)
	if !ok || text == "" {
		return nil, badParam("query is text and not empty", "query")
	}
	if utf8.RuneCountInString(text) > searchQueryMax {
		return nil, badParam(fmt.Sprintf("query is at most %d characters", searchQueryMax), "query")
	}
	limit, err := limitOr(params, searchHitsMax, searchHitsDefault)
	if err != nil {
		return nil, err
	}
	like, hits, more := likeOf(text), []any{}, false
	// One more than the limit is read of each kind, which says whether more remain.
	cut := func(rs []row) []row {
		if len(rs) > limit {
			more = true
			return rs[:limit]
		}
		return rs
	}
	tasks, err := query(e.DB, searchTasksSQL, like, like, like, like, limit+1)
	if err != nil {
		return nil, err
	}
	for _, t := range cut(tasks) {
		hits = append(hits, taskHit(t, text))
	}
	memos, err := query(e.DB, searchCommentsSQL, like, limit+1)
	if err != nil {
		return nil, err
	}
	for _, m := range cut(memos) {
		hits = append(hits, map[string]any{"kind": "comment", "at": ts(m["created_at"]), "snippet": snippetOf(m.str("body"), text),
			"task": m.i("task_id"), "title": m.str("title"), "memo": m.i("id"), "from": m.str("author")})
	}
	messages, err := query(e.DB, searchMessagesSQL, like, limit+1)
	if err != nil {
		return nil, err
	}
	for _, r := range cut(messages) {
		hit, err := e.messageHit(r, text)
		if err != nil {
			return nil, err
		}
		if hit != nil {
			hits = append(hits, hit)
		}
	}
	return map[string]any{"hits": hits, "more": more}, nil
}

// taskHit is a task as a hit, by the first of its title, brief and result that holds the text.
func taskHit(t row, text string) map[string]any {
	field, found := "title", t.str("title")
	switch {
	case holds(found, text):
	case holds(t.str("brief"), text):
		field, found = "brief", t.str("brief")
	case holds(t.str("done_evidence"), text):
		field, found = "result", t.str("done_evidence")
	default:
		field, found = "result", t.str("report")
	}
	return map[string]any{"kind": "task", "at": ts(t["created_at"]), "snippet": snippetOf(found, text), "field": field,
		"task": t.i("id"), "title": t.str("title")}
}

// messageHit is a request as a hit with the chat that shows it, as chat.search finds it for a person; nil when no chat does (a
// message between people).
func (e *Env) messageHit(r row, text string) (map[string]any, error) {
	found, err := e.locateMessage(r.i("id"), ownerOfChat(r), false)
	if err != nil {
		return nil, err
	}
	if found.kind == "alone" {
		return nil, nil
	}
	hit := map[string]any{"kind": "message", "at": ts(r["ts"]), "snippet": snippetOf(r.str("body"), text), "request": r.i("id"),
		"agent": found.agent, "scope": found.tab, "from": r.str("requester")}
	if r["task_id"] != nil {
		hit["task"], hit["title"] = r.i("task_id"), r.str("task_title")
	}
	return hit, nil
}
