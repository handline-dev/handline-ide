package team

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

// Temps (go/internal/temps): the rows of a temp, a seat of an agent that takes a task that came to it while busy. A seat stays once its task
// is over, idle (agents.hired_for unset), for the agent's next hire.

// notCopied are the agents columns a temp does not take from the agent it is hired from: its own identity, and the agent's schedules,
// which would run twice.
var notCopied = []string{"id", "name", "created_at", "hired_from", "hired_for", "schedules"}

// NewSeat adds a seat of agent origin (its key), idle, and is its key. It takes every setting of origin's row; its number is the lowest
// no agent of the workspace is called by.
func NewSeat(q db.Q, origin string, now int64) (string, error) {
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

// nextTempNumber is the lowest number from 2 that no agent of workspace ws is called name-<n> by.
func nextTempNumber(q db.Q, ws int64, name string) (int, error) {
	rows, err := q.Query("SELECT name FROM agents WHERE workspace = ?", ws)
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

// SetHiredFor marks seat, idle, at work on task tid, or idle with tid 0; an error when seat is no seat or, for a tid, not idle.
func SetHiredFor(q db.Q, seat string, tid int64) error {
	_, id, _ := actor.Parse(seat)
	query, args := "UPDATE agents SET hired_for = NULL WHERE id = ? AND hired_from IS NOT NULL", []any{id}
	if tid != 0 {
		query, args = "UPDATE agents SET hired_for = ? WHERE id = ? AND hired_from IS NOT NULL AND hired_for IS NULL", []any{tid, id}
	}
	res, err := q.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return cmp.Or(err, fmt.Errorf("%s is no idle seat", seat))
	}
	return nil
}

// HiredFor is the task seat is at work on, 0 while it is idle.
func HiredFor(q db.Q, seat string) (int64, error) {
	_, id, _ := actor.Parse(seat)
	var tid sql.NullInt64
	err := q.QueryRow("SELECT hired_for FROM agents WHERE id = ?", id).Scan(&tid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return tid.Int64, err
}

// Holds is whether seat temp still has work: an open task it acts on or owns (one with the founder's or a checker's turn on top is still its),
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

// TempsOf is the keys of the seats of agent, lowest number first (seats are numbered as they are made).
func TempsOf(q db.Q, agent string) ([]string, error) {
	rows, err := q.Query("SELECT id FROM agents WHERE hired_from = ? ORDER BY id", agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, actor.Key(actor.Agent, id))
	}
	return out, rows.Err()
}
