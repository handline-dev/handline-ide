package taskwrites

import (
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/protocol/rpc"
)

// RegisterReviewLevels serves how much of an agent's work of each kind the founder looks at: the founder sets it, the app reads it.
func RegisterReviewLevels(s *rpc.Server, home string) {
	s.Register("agent.review_set", rpc.Method{Handler: reviewSet(home)})
	s.Register("agent.review", rpc.Method{Handler: reviewGet(home), Read: true})
}

func agentParam(p map[string]any) (string, error) {
	a, _ := p["agent"].(string)
	if actor.KindOf(a) != actor.Agent {
		return "", badParam("agent names an agent", "agent")
	}
	return a, nil
}

func reviewSet(home string) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		agent, err := agentParam(p)
		if err != nil {
			return nil, err
		}
		work, _ := p["work"].(string)
		if !slices.Contains(db.Works, work) {
			return nil, badParam("work is contract, screen, daemon or docs", "work")
		}
		level, _ := p["level"].(string)
		if !slices.Contains(db.Levels, level) {
			return nil, badParam("level is every, one_in_two, one_in_three, one_in_five or never", "level")
		}
		if work == db.WorkContract && level != db.LevelEvery {
			return nil, refusal("contract and data work is always looked at: its level stays every")
		}
		me := c.Caller.Actor()
		return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			model, err := team.NewRosterOn(tx, home).Model(agent)
			if err != nil {
				return nil, err
			}
			now, err := db.ReviewLevel(tx, agent, work, model)
			if err != nil {
				return nil, err
			}
			if now.Level == level {
				return accepted(fmt.Sprintf("%s's %s work is checked at %s already", addr.Ref(agent), work, level), nil), nil
			}
			if err := db.SetReviewLevel(tx, agent, work, model, level, me, "set", 0); err != nil {
				return nil, err
			}
			return accepted(fmt.Sprintf("%s's %s work is checked at %s from now", addr.Ref(agent), work, level), nil), nil
		})
	}
}

func reviewGet(home string) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		agent, err := agentParam(p)
		if err != nil {
			return nil, err
		}
		q := c.Server().DB
		model, err := team.NewRosterOn(q, home).Model(agent)
		if err != nil {
			return nil, err
		}
		works, err := db.AgentWorks(q, agent)
		if err != nil {
			return nil, err
		}
		ladder := make([]any, 0, len(works))
		for _, w := range works {
			l, err := db.ReviewLevel(q, agent, w, model)
			if err != nil {
				return nil, err
			}
			rec, err := db.ReviewRecord(q, agent, model, l)
			if err != nil {
				return nil, err
			}
			last := rec[max(0, len(rec)-db.StepAfter):]
			looks := make([]any, 0, len(last))
			for _, k := range last {
				looks = append(looks, map[string]any{"task": k.Task, "ok_first": k.OKFirst, "at": time.Unix(k.At, 0).UTC().Format(time.RFC3339)})
			}
			var next any
			if yes, _ := db.LearnedEnough(l, rec); yes {
				next = db.LevelDown(l.Level)
			}
			ladder = append(ladder, map[string]any{"work": w, "level": l.Level, "record": looks, "looks": len(rec), "suggested": next})
		}
		lessons, err := db.LiveLessons(q, agent)
		if err != nil {
			return nil, err
		}
		return map[string]any{"agent": agent, "ladder": ladder, "lessons": len(lessons)}, nil
	}
}
