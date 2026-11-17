package taskwrites

import (
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/protocol/rpc"
)

// RegisterReviewLevels serves how much of an agent's story work the founder looks at: the founder sets it, the app reads it.
func RegisterReviewLevels(s *rpc.Server) {
	s.Register("agent.review_set", rpc.Method{Handler: reviewSet})
	s.Register("agent.review", rpc.Method{Handler: reviewGet, Read: true})
}

func agentParam(p map[string]any) (string, error) {
	a, _ := p["agent"].(string)
	if actor.KindOf(a) != actor.Agent {
		return "", badParam("agent names an agent", "agent")
	}
	return a, nil
}

func reviewSet(c *rpc.Conn, p map[string]any) (any, error) {
	agent, err := agentParam(p)
	if err != nil {
		return nil, err
	}
	level, _ := p["level"].(string)
	if !slices.Contains(db.Levels, level) {
		return nil, badParam("level is every, one_in_three or auto", "level")
	}
	me := c.Caller.Actor()
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		now, err := db.ReviewLevel(tx, agent)
		if err != nil {
			return nil, err
		}
		if now.Level == level {
			return accepted(fmt.Sprintf("%s is checked at %s already", addr.Ref(agent), level), nil), nil
		}
		if err := db.SetReviewLevel(tx, agent, level, me, "set", 0); err != nil {
			return nil, err
		}
		return accepted(fmt.Sprintf("%s is checked at %s from now", addr.Ref(agent), level), nil), nil
	})
}

func reviewGet(c *rpc.Conn, p map[string]any) (any, error) {
	agent, err := agentParam(p)
	if err != nil {
		return nil, err
	}
	q := c.Server().DB
	l, err := db.ReviewLevel(q, agent)
	if err != nil {
		return nil, err
	}
	rec, err := db.ReviewRecord(q, agent, l)
	if err != nil {
		return nil, err
	}
	lessons, err := db.LiveLessons(q, agent)
	if err != nil {
		return nil, err
	}
	last := rec[max(0, len(rec)-10):]
	looks := make([]any, 0, len(last))
	for _, k := range last {
		looks = append(looks, map[string]any{"task": k.Task, "ok_first": k.OKFirst, "at": time.Unix(k.At, 0).UTC().Format(time.RFC3339)})
	}
	var next any
	if yes, _ := db.LearnedEnough(l.Level, rec); yes {
		next = db.LevelDown(l.Level)
	}
	return map[string]any{"agent": agent, "level": l.Level, "record": looks, "looks": len(rec), "suggested": next, "lessons": len(lessons)}, nil
}
