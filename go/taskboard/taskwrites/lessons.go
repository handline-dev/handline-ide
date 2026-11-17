package taskwrites

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// lessonMost is the most bytes one lesson takes, so no one lesson fills the cap.
const lessonMost = 500

// RegisterLessons serves what agents learned: an agent adds and merges its own, anyone reads them, the owner removes one.
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
func fullRefusal() error {
	return refusal(fmt.Sprintf("your lessons are at their %d-byte limit: merge similar ones with `hl lesson merge <ids> \"<text>\"` "+
		"(`hl lesson list` shows their ids), then add this one", db.LessonsCap))
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
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
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
		id, err := db.AddLesson(tx, me, s, tid, me)
		if errors.Is(err, db.ErrLessonsFull) {
			return nil, fullRefusal()
		}
		if err != nil {
			return nil, err
		}
		return accepted(fmt.Sprintf("Lesson %d kept: your next session starts with it", id), map[string]any{"id": id}), nil
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
		return nil, badParam("ids are two or more different lessons of yours", "ids")
	}
	me := c.Caller.Actor()
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		id, err := db.MergeLessons(tx, me, ids, s, me)
		var notLive db.NotLiveLesson
		switch {
		case errors.Is(err, db.ErrLessonsFull):
			return nil, fullRefusal()
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
	live, err := db.LiveLessons(c.Server().DB, agent)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(live))
	for _, l := range live {
		it := map[string]any{"id": l.ID, "agent": l.Agent, "text": l.Text, "created_by": l.CreatedBy,
			"created_at": time.Unix(l.CreatedAt, 0).UTC().Format(time.RFC3339)}
		if l.Task != 0 {
			it["task"] = l.Task
		}
		items = append(items, it)
	}
	return map[string]any{"agent": agent, "items": items, "bytes": len(db.LessonsSection(live)), "cap": db.LessonsCap}, nil
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
