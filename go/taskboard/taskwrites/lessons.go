package taskwrites

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/requestref"
	"github.com/handline/handline/go/taskboard/rulesfold"
)

// lessonMost is the most bytes one lesson takes, so no one lesson fills the cap.
const lessonMost = 500

// RegisterLessons serves what agents learned: an agent adds and merges its own and the founder's (about: founder), anyone reads them, the
// owner removes one.
func RegisterLessons(s *rpc.Server) {
	s.Register("lesson.add", rpc.Method{Handler: lessonAdd})
	s.Register("lesson.merge", rpc.Method{Handler: lessonMerge})
	s.Register("lessons.list", rpc.Method{Handler: lessonsList, Read: true})
	s.Register("lesson.remove", rpc.Method{Handler: lessonRemove})
}

func lessonText(p map[string]any) (string, error) {
	s, err := text(p, "text")
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	if len(s) > lessonMost || textsafe.HasUnsafe(s) {
		return "", badParam(fmt.Sprintf("text is one line of plain text, at most %d bytes", lessonMost), "text")
	}
	return s, nil
}

// fullRefusal is the add or merge past the cap, saying how to make room.
func fullRefusal(founder bool) error {
	whose, limit, merge, list := "your lessons are", db.LessonsCap, `hl lesson merge <ids> "<text>"`, "hl lesson list"
	if founder {
		whose, limit = "the founder's preferences are", db.PersonCap
		merge, list = `hl lesson merge <ids> "<text>" --about founder --said <comment|msg>`, "hl lesson list founder"
	}
	return refusal(fmt.Sprintf("%s at their %d-byte limit: merge similar ones with `%s` (`%s` shows their ids), then add this one",
		whose, limit, merge, list))
}

// aboutFounder is whether a lesson write is about the caller's founder (about: founder) rather than its own.
func aboutFounder(p map[string]any) (bool, error) {
	switch about, _ := p["about"].(string); about {
	case "":
		return false, nil
	case "founder":
		return true, nil
	default:
		return false, badParam("about is founder, or left out for your own lesson", "about")
	}
}

// whose is the key a lesson write is kept under: the caller's, or its owner's for one about the founder.
func whose(tx *sql.Tx, me string, founder bool) (string, error) {
	if !founder {
		return paths.KnowsAs(tx, me) // a temp's lessons are the agent's it was hired from
	}
	return addr.OwnerOf(tx, me)
}

// cited is text with the founder's words it comes from named after it: said is one of their comments (its number) or messages (msg_<hash>).
// Without said, or with one the founder did not write, a lesson about them is refused: who said it must be kept.
func cited(tx *sql.Tx, p map[string]any, text, me, founder string) (string, error) {
	v, ok := p["said"]
	if !ok || v == nil {
		return "", badParam("said names the founder's own words this comes from: a comment number or a msg_<hash>", "said")
	}
	if n, ok := rpc.AsInt(v); ok {
		m, err := db.GetComment(tx, n)
		if err != nil {
			return "", err
		}
		if m == nil || m.Author != founder {
			return "", refusal(fmt.Sprintf("comment %d is not the founder's: cite a comment or message the founder wrote", n))
		}
		return fmt.Sprintf("%s (the founder, comment %d)", text, n), nil
	}
	rid, err := requestref.Resolve(tx, v, me, "said")
	if err != nil {
		return "", err
	}
	var requester string
	if err := tx.QueryRow("SELECT requester FROM requests WHERE id = ?", rid).Scan(&requester); err != nil {
		return "", err
	}
	if requester != founder {
		return "", refusal(fmt.Sprintf("%v is not the founder's: cite a comment or message the founder wrote", v))
	}
	return fmt.Sprintf("%s (the founder, %s)", text, db.MsgName(tx, rid)), nil
}

func lessonAdd(c *rpc.Conn, p map[string]any) (any, error) {
	s, err := lessonText(p)
	if err != nil {
		return nil, err
	}
	tid, err := taskID(p)
	if err != nil {
		return nil, err
	}
	me := c.Caller.Actor()
	founder, err := aboutFounder(p)
	if err != nil {
		return nil, err
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		agent, err := whose(tx, me, founder)
		if err != nil {
			return nil, err
		}
		t, err := getTask(tx, tid)
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, refusal(fmt.Sprintf("no task T%d", tid))
		}
		if t.Owner != me && t.Acting != me {
			return nil, refusal(fmt.Sprintf("T%d is not your task: keep a lesson on a task you work", tid))
		}
		if founder {
			if s, err = cited(tx, p, s, me, agent); err != nil {
				return nil, err
			}
		}
		id, err := db.AddLesson(tx, agent, s, tid, me)
		if errors.Is(err, db.ErrLessonsFull) {
			return nil, fullRefusal(founder)
		}
		if err != nil {
			return nil, err
		}
		next := "your next session starts with it"
		if founder {
			next = "every agent's next session starts with it"
		}
		return accepted(fmt.Sprintf("Lesson %d kept: %s", id, next), map[string]any{"id": id}), nil
	})
}

func lessonMerge(c *rpc.Conn, p map[string]any) (any, error) {
	s, err := lessonText(p)
	if err != nil {
		return nil, err
	}
	raw, _ := p["ids"].([]any)
	var ids []int64
	for _, v := range raw {
		n, ok := rpc.AsInt(v)
		if !ok {
			return nil, badParam("ids are lesson numbers", "ids")
		}
		if !slices.Contains(ids, n) {
			ids = append(ids, n)
		}
	}
	if len(ids) < 2 {
		return nil, badParam("ids are two or more different lessons (yours, or the founder's with about)", "ids")
	}
	me := c.Caller.Actor()
	founder, err := aboutFounder(p)
	if err != nil {
		return nil, err
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		agent, err := whose(tx, me, founder)
		if err != nil {
			return nil, err
		}
		if founder {
			if s, err = cited(tx, p, s, me, agent); err != nil {
				return nil, err
			}
		}
		id, err := db.MergeLessons(tx, agent, ids, s, me)
		var notLive db.NotLiveLesson
		switch {
		case errors.Is(err, db.ErrLessonsFull):
			return nil, fullRefusal(founder)
		case errors.As(err, &notLive):
			return nil, refusal(err.Error())
		case err != nil:
			return nil, err
		}
		return accepted(fmt.Sprintf("Lessons %s merged into lesson %d", joinIDs(ids), id), map[string]any{"id": id}), nil
	})
}

func lessonsList(c *rpc.Conn, p map[string]any) (any, error) {
	agent, _ := p["agent"].(string)
	if agent == "" {
		if c.Caller.Role != "agent" {
			return nil, badParam("agent names whose lessons", "agent")
		}
		agent = c.Caller.Actor()
	}
	agent, err := paths.KnowsAs(c.Server().DB, agent)
	if err != nil {
		return nil, err
	}
	live, err := db.LiveLessons(c.Server().DB, agent)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(live))
	for _, l := range live {
		it := map[string]any{"id": l.ID, "agent": l.Agent, "text": l.Text, "count": 1, "created_by": l.CreatedBy,
			"created_at": time.Unix(l.CreatedAt, 0).UTC().Format(time.RFC3339)}
		if l.Task != 0 {
			it["task"] = l.Task
		}
		if actor.IsPerson(agent) {
			ruleView(it, rulesfold.FromLesson(l))
		}
		items = append(items, it)
	}
	limit := db.LessonsCap
	if actor.IsPerson(agent) {
		limit = db.PersonCap
	}
	return map[string]any{"agent": agent, "items": items, "bytes": len(db.Section(agent, live)), "cap": limit}, nil
}

// ruleView puts a person's rule into its lesson's row: the rule alone as text, how often it was said, and the one quote kept,
// with the comment or chat message it was said in.
func ruleView(it map[string]any, r rulesfold.Rule) {
	it["text"], it["count"] = r.Text, r.Count
	if r.QuoteRef == "" {
		return
	}
	q := map[string]any{"text": r.Quote}
	if r.Task != 0 {
		q["task"] = r.Task
	}
	if n, ok := strings.CutPrefix(r.QuoteRef, "comment "); ok {
		if id, err := strconv.ParseInt(n, 10, 64); err == nil {
			q["comment"] = id
		}
	} else if strings.HasPrefix(r.QuoteRef, db.MsgPrefix) {
		q["message"] = r.QuoteRef
	}
	it["quote"] = q
}

func lessonRemove(c *rpc.Conn, p map[string]any) (any, error) {
	id, ok := rpc.AsInt(p["id"])
	if !ok {
		return nil, badParam("id is a lesson number", "id")
	}
	me := c.Caller.Actor()
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		if gone, err := db.RemoveLesson(tx, id, me); err != nil {
			return nil, err
		} else if !gone {
			e := rpc.NotFoundError(fmt.Sprintf("no lesson %d", id))
			e.Data = map[string]any{"what": "lesson"}
			return nil, e
		}
		return accepted(fmt.Sprintf("Lesson %d removed", id), nil), nil
	})
}

func joinIDs(ids []int64) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = fmt.Sprint(id)
	}
	return strings.Join(s, ", ")
}
