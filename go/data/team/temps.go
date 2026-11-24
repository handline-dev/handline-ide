package team

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
)

// Temps (go/internal/temps): the rows of a temp, hired for a task that came to a busy agent.

// notCopied are the agents columns a temp does not take from the agent it is hired from: its own identity, and the agent's schedules,
// which would run twice.
var notCopied = []string{"id", "name", "created_at", "hired_from", "schedules"}

// HireRow adds a temp of agent origin (its key; a temp's own origin when origin is a temp) and is the temp's key. The temp takes every
// setting of origin's row; its number is the lowest no agent or temp of the workspace has had.
func HireRow(q db.Q, origin string, now int64) (string, error) {
	if from, err := paths.HiredFrom(q, origin); err != nil {
		return "", err
	} else if from != "" {
		origin = from
	}
	_, id, ok := actor.Parse(origin)
	if !ok {
		return "", fmt.Errorf("%s is not an agent's key", origin)
	}
	var ws int64
	var name string
	if err := q.QueryRow("SELECT workspace, name FROM agents WHERE id = ?", id).Scan(&ws, &name); err != nil {
		return "", err
	}
	n, err := nextTempNumber(q, ws, name)
	if err != nil {
		return "", err
	}
	cols, err := agentColumns(q)
	if err != nil {
		return "", err
	}
	copied := slices.DeleteFunc(cols, func(c string) bool { return slices.Contains(notCopied, c) })
	list := strings.Join(copied, ", ")
	res, err := q.Exec("INSERT INTO agents(name, created_at, hired_from, "+list+") SELECT ?, ?, ?, "+list+" FROM agents WHERE id = ?",
		name+"-"+strconv.Itoa(n), now, origin, id)
	if err != nil {
		return "", err
	}
	made, err := res.LastInsertId()
	return actor.Key(actor.Agent, made), err
}

// nextTempNumber is the lowest number from 2 that no agent or left temp of workspace ws is called name-<n> by, so a name a temp had
// is never another's: what is still addressed to it goes to the agent it was hired from.
func nextTempNumber(q db.Q, ws int64, name string) (int, error) {
	rows, err := q.Query("SELECT name FROM agents WHERE workspace = ?1 UNION SELECT name FROM temps_left WHERE workspace = ?1", ws)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	used := map[int]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
		if rest, ok := strings.CutPrefix(n, name+"-"); ok {
			if i, err := strconv.Atoi(rest); err == nil {
				used[i] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 2
	for used[n] {
		n++
	}
	return n, nil
}

func agentColumns(q db.Q) ([]string, error) {
	rows, err := q.Query("SELECT name FROM pragma_table_info('agents')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Temps are the temps on the roster, by key, each with the key of the agent it was hired from.
func Temps(q db.Q) (map[string]string, error) {
	rows, err := q.Query("SELECT id, hired_from FROM agents WHERE hired_from IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id int64
		var from string
		if err := rows.Scan(&id, &from); err != nil {
			return nil, err
		}
		out[actor.Key(actor.Agent, id)] = from
	}
	return out, rows.Err()
}

// LeftRows is temp's rows as it leaves, before its agents row goes: what names it (a task's owner, opener, reporter or turn, a request
// queued to it or one it asked) names the agent it was hired from instead, and temps_left keeps its name for words that still address
// it. The agent it was hired from is the answer.
func LeftRows(q db.Q, temp string, now int64) (string, error) {
	from, err := paths.HiredFrom(q, temp)
	if err != nil || from == "" {
		return "", err
	}
	_, id, _ := actor.Parse(temp)
	if _, err := q.Exec(`INSERT OR REPLACE INTO temps_left(agent, workspace, name, hired_from, left_at)
		SELECT ?, workspace, name, hired_from, ? FROM agents WHERE id = ?`, temp, now, id); err != nil {
		return "", err
	}
	if _, err := q.Exec("UPDATE requests SET target = ? WHERE target = ? AND state = 'queued'", from, temp); err != nil {
		return "", err
	}
	for _, at := range []string{"tasks.owner", "tasks.opened_by", "tasks.reporter", "tasks.queued_by", "task_turns.agent", "requests.requester"} {
		table, col, _ := strings.Cut(at, ".")
		if _, err := q.Exec("UPDATE "+table+" SET "+col+" = ? WHERE "+col+" = ?", from, temp); err != nil {
			return "", err
		}
	}
	return from, nil
}

// Holds is whether temp still has work: an open task it acts on or owns (one with the founder's or a checker's turn on top is still its),
// or a request queued to it.
func Holds(q db.Q, temp string) (bool, error) {
	var n int
	if err := q.QueryRow("SELECT COUNT(*) FROM tasks WHERE owner = ? AND "+db.OpenSQL, temp).Scan(&n); err != nil || n > 0 {
		return n > 0, err
	}
	if open, err := db.OpenTasksOf(q, temp); err != nil || len(open) > 0 {
		return len(open) > 0, err
	}
	queued, err := db.QueuedFor(q, temp)
	return len(queued) > 0, err
}

// TempsOf is the keys of the temps hired from agent.
func TempsOf(q db.Q, agent string) ([]string, error) {
	all, err := Temps(q)
	var out []string
	for t, from := range all {
		if from == agent {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out, err
}
