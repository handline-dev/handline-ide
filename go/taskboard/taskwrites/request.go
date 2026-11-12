package taskwrites

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/kit/addr"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/attachments"
	"github.com/handline/handline/go/taskboard/requestref"
)

// titleMax is the longest title task.request takes from a message's first line.
const titleMax = 80

// founderMessage is the founder's message to an agent that task.request acts on: the one who may act on it is the founder or
// that agent. A message that is not so is refused, naming why.
func founderMessage(tx *sql.Tx, home string, c *rpc.Conn, p map[string]any) (int64, *db.Request, error) {
	me := c.Caller.Actor()
	rid, err := requestref.Resolve(tx, p["request"], me, "request")
	if name, missing := requestref.Missing(err); missing {
		return 0, nil, refusal("no request #" + name)
	}
	if err != nil {
		return 0, nil, err
	}
	req, err := db.GetRequest(tx, rid)
	if err != nil {
		return 0, nil, err
	}
	name := db.MsgName(tx, rid)
	switch {
	case !actor.IsPerson(req.Requester) || actor.IsPerson(req.Target):
		return 0, nil, refusal(fmt.Sprintf("%s is not a message from the founder to an agent", name))
	case me != actor.OwnerKey && me != req.Target:
		return 0, nil, refusal(fmt.Sprintf("%s was sent to %s: only it, or the founder, may act on it", name, addr.Ref(req.Target)))
	}
	return rid, req, nil
}

// requestTask is the task made from message rid: a request task (from_request), or one begun on the message; 0 for none.
func requestTask(tx *sql.Tx, rid int64) (tid int64, err error) {
	err = tx.QueryRow("SELECT COALESCE((SELECT id FROM tasks WHERE from_request = ? ORDER BY id LIMIT 1), "+
		"(SELECT task_id FROM task_requests WHERE request_id = ? ORDER BY task_id LIMIT 1), 0)", rid, rid).Scan(&tid)
	return tid, err
}

// requestMethod is task.request: a founder's message becomes a task the founder opened (opened_by human), held by the lead it was sent to,
// with the message as its first comment. A message converts once.
func requestMethod(home string) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		owner, title := "", ""
		if p["owner"] != nil {
			var err error
			if owner, err = text(p, "owner"); err != nil {
				return nil, err
			}
			if !contains(paths.ListAgents(home), owner) {
				return nil, agentNotFound(owner)
			}
		}
		if p["title"] != nil {
			var err error
			if title, err = text(p, "title"); err != nil {
				return nil, err
			}
		}
		return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			rid, req, err := founderMessage(tx, home, c, p)
			if err != nil {
				return nil, err
			}
			name := db.MsgName(tx, rid)
			if tid, err := requestTask(tx, rid); err != nil {
				return nil, err
			} else if tid != 0 {
				return accepted(fmt.Sprintf("%s is already T%d", name, tid), map[string]any{"task": tid, "existing": true}), nil
			}
			if owner == "" {
				owner = req.Target
			}
			clean := strings.TrimSpace(textsafe.Oneline(title))
			if clean == "" {
				clean = headline(req.Body)
			}
			tid, err := db.NewTask(tx, clean, owner, req.Requester, 0, db.DefaultPriority, "report", 0, "")
			if err != nil {
				return nil, err
			}
			if _, err := tx.Exec("UPDATE tasks SET from_request = ? WHERE id = ?", rid, tid); err != nil {
				return nil, err
			}
			if err := db.LinkRequest(tx, tid, rid); err != nil {
				return nil, err
			}
			if _, err := tx.Exec("UPDATE requests SET state_at = ? WHERE id = ?", db.Now(), rid); err != nil {
				return nil, err
			}
			if _, err := db.SetTaskQueued(tx, tid, req.Requester); err != nil { // the founder's own task needs no one's approval
				return nil, err
			}
			if _, held, err := db.CurrentTask(tx, owner); err != nil {
				return nil, err
			} else if !held {
				if err := db.SetCurrentTask(tx, owner, tid, true); err != nil {
					return nil, err
				}
			}
			files, err := attachments.Of(tx, rid)
			if err != nil {
				return nil, err
			}
			// A ref is a path, not a kept file: the memo says it in words, numbered after the files.
			memo := strings.Join(append([]string{req.Body}, attachments.PathRefLines(attachments.PathRefsOf(tx, rid), len(files))...), "\n")
			mid, err := db.AddMemo(tx, tid, req.Requester, strings.TrimSpace(memo))
			if err != nil {
				return nil, err
			}
			if err := attachments.New(home).AttachMemo(tx, mid, files); err != nil {
				return nil, err
			}
			if err := addEvent(tx, c.Caller.Actor(), "task_request", kv{"task_id", tid}, kv{"chat_request", rid}, kv{"owner", owner}); err != nil {
				return nil, err
			}
			return accepted(fmt.Sprintf("%s is now request T%d, held by %s", name, tid, addr.Ref(owner)), map[string]any{"task": tid}), nil
		})
	}
}

// headline is a message's first line, cut to a title.
func headline(body string) string {
	line := strings.TrimSpace(textsafe.Oneline(strings.SplitN(strings.TrimSpace(body), "\n", 2)[0]))
	if r := []rune(line); len(r) > titleMax {
		line = strings.TrimSpace(string(r[:titleMax-1])) + "…"
	}
	if line == "" {
		return "(a message without words)"
	}
	return line
}
