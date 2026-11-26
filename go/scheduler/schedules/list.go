package schedules

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/cronspec"
)

// What is scheduled and when it ran, for a person to read: schedules.list. It reads what the runner reads (each agent's `schedules`
// value, the cursor a schedule was last handled at) and what the runner wrote (the messages it typed), and writes nothing.

const (
	// MessageMax is how much of a schedule's message a row carries, in characters: what a proposal may ask for at most.
	MessageMax = 1000
	// FiresMax is how many of a schedule's newest messages a row carries.
	FiresMax = 5
)

// Body is the message the runner types for a schedule. A message with this text, sent by Requester to the agent, is one time it fired:
// the text holds the cron and the message, which are what a schedule is.
func Body(it cronspec.Schedule) string { return fmt.Sprintf("[schedule %s] %s", it.Cron, it.Message) }

// Fire is one time a schedule's message was typed: the request it was written as.
type Fire struct {
	At      int64
	Request int64
	Ref     string
}

// Row is one schedule of an agent as schedules.list gives it.
type Row struct {
	Agent    string
	Schedule cronspec.Schedule
	// Next is the first time of the expression after the minute the runner last handled it (after now, for one it has not seen yet),
	// 0 when the expression names no time within eight years. Due says that time has come and the message waits for the agent's turn to end.
	Next int64
	Due  bool
	// Fires are its newest messages, newest first, at most FiresMax.
	Fires []Fire
}

// Unreadable is an agent whose `schedules` value is wrong: the runner sends none of it.
type Unreadable struct{ Agent, Why string }

// List is the schedules of agents in the order given, each agent's as its value keeps them. load is cronspec.Load.
func List(q db.Q, agents []string, load func(agent string) ([]cronspec.Schedule, error), now int64) ([]Row, []Unreadable, error) {
	rows, bad := []Row{}, []Unreadable{}
	for _, agent := range agents {
		items, err := load(agent)
		var wrong *cronspec.ScheduleError
		if errors.As(err, &wrong) {
			bad = append(bad, Unreadable{agent, wrong.Msg})
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		for _, it := range items {
			r := Row{Agent: agent, Schedule: it}
			if r.Next, r.Due, err = next(q, agent, it, now); err != nil {
				return nil, nil, err
			}
			if r.Fires, err = fires(q, agent, it); err != nil {
				return nil, nil, err
			}
			rows = append(rows, r)
		}
	}
	return rows, bad, nil
}

// next is when the runner sends it next, by the runner's own rule (Runner.agent): the first time of the expression after its cursor,
// and for a schedule with no cursor yet after this minute, where its first sweep puts the cursor.
func next(q db.Q, agent string, it cronspec.Schedule, now int64) (int64, bool, error) {
	handled, err := db.Cursor(q, "schedule_at:"+agent+":"+cronspec.SID(it))
	if err != nil {
		return 0, false, err
	}
	if handled == 0 {
		handled = now - now%60
	}
	at, ok := cronspec.NextCronAfter(it.Cron, handled)
	if !ok {
		return 0, false, nil
	}
	return at, at <= now, nil
}

func fires(q db.Q, agent string, it cronspec.Schedule) ([]Fire, error) {
	rows, err := q.Query("SELECT id, ts, COALESCE(ref, '') FROM requests WHERE requester = ? AND target = ? AND body = ? "+
		"ORDER BY ts DESC, id DESC LIMIT ?", Requester, agent, Body(it), FiresMax)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Fire{}
	for rows.Next() {
		var f Fire
		if err := rows.Scan(&f.Request, &f.At, &f.Ref); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Register serves schedules.list over the agents of home.
func Register(s *rpc.Server, home string) {
	s.Register("schedules.list", rpc.Method{Read: true, Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		return answer(c.Server().DB, home, p, time.Now())
	}})
}

// answer is schedules.list's result at now: every agent's schedules, or with the agent param (its key) that agent's.
func answer(q db.Q, home string, p map[string]any, now time.Time) (any, error) {
	all := paths.ListAgents(home)
	if agent, _ := p["agent"].(string); agent != "" {
		if !slices.Contains(all, agent) {
			e := rpc.NotFoundError("no such agent: " + agent)
			e.Data = map[string]any{"what": "agent", "name": agent}
			return nil, e
		}
		all = []string{agent}
	}
	load := func(agent string) ([]cronspec.Schedule, error) { return cronspec.Load(home, agent) }
	rows, bad, err := List(q, all, load, now.Unix())
	if err != nil {
		return nil, err
	}
	return reply(rows, bad, now), nil
}

// reply is the method's result: the rows as the contract's Schedule, the agents whose value is wrong, and the machine's zone at now.
func reply(rows []Row, bad []Unreadable, now time.Time) map[string]any {
	list := make([]any, 0, len(rows))
	for _, r := range rows {
		message, more := cut(r.Schedule.Message, MessageMax)
		row := map[string]any{"id": cronspec.SID(r.Schedule), "agent": r.Agent, "cron": r.Schedule.Cron, "sentence": nil, "message": message,
			"message_more": more, "next": nil, "due": r.Due}
		if said, ok := cronspec.Describe(r.Schedule.Cron); ok {
			row["sentence"] = said
		}
		if r.Next != 0 {
			row["next"] = stamp(r.Next)
		}
		fired := make([]any, 0, len(r.Fires))
		for _, f := range r.Fires {
			fired = append(fired, map[string]any{"at": stamp(f.At), "request": f.Request, "ref": f.Ref})
		}
		row["fires"] = fired
		list = append(list, row)
	}
	unreadable := make([]any, 0, len(bad))
	for _, b := range bad {
		unreadable = append(unreadable, map[string]any{"agent": b.Agent, "why": b.Why})
	}
	name, offset := now.In(time.Local).Zone()
	return map[string]any{"schedules": list, "unreadable": unreadable, "zone": map[string]any{"name": name, "offset_min": offset / 60}}
}

// cut is text held to max characters, and whether it had more.
func cut(text string, max int) (string, bool) {
	if utf8.RuneCountInString(text) <= max {
		return text, false
	}
	return strings.TrimSpace(string([]rune(text)[:max])), true
}

// stamp is the spec's Timestamp: whole seconds, UTC.
func stamp(at int64) string { return time.Unix(at, 0).UTC().Format("2006-01-02T15:04:05Z") }
