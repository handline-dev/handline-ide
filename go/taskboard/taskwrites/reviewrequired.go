package taskwrites

import (
	"database/sql"
	"fmt"
	"slices"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
)

// reviewMissing is why a done with commits is refused under the closing agent's workspace's review_required, "" when no such rule is
// on or the task has a review: a turn on it answered with a comment by an agent that neither closes it, holds it nor ever handed it on.
func reviewMissing(tx *sql.Tx, by string, t *task) (string, error) {
	rules, _, err := team.AgentRules(tx, by)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(rules, func(r team.KeptRule) bool { return r.Key == team.RuleReviewRequired && team.RuleOn(r.Value) })
	if i < 0 {
		return "", nil
	}
	workers := []string{by, t.Owner, t.Acting}
	from, err := column(tx, "SELECT json_extract(payload, '$.from') FROM events WHERE kind = 'task_handoff' AND json_valid(payload) "+
		"AND json_extract(payload, '$.task_id') = ?", t.ID)
	if err != nil {
		return "", err
	}
	reviewers, err := column(tx, "SELECT json_extract(payload, '$.agent') FROM events WHERE kind = 'task_turn_pop' AND json_valid(payload) "+
		"AND json_extract(payload, '$.task_id') = ? AND json_extract(payload, '$.memo') IS NOT NULL", t.ID)
	if err != nil {
		return "", err
	}
	workers = append(workers, from...)
	for _, a := range reviewers {
		if k, _, ok := actor.Parse(a); ok && k == actor.Agent && !slices.Contains(workers, a) {
			return "", nil
		}
	}
	return fmt.Sprintf("task T%d has commits, and its workspace's rule %s wants a review first: give an agent that did not work it "+
		"a turn (hl task comment T%d add \"...\" --turn <agent>); its answer is the review", t.ID, rules[i].Words(), t.ID), nil
}

// column is the one text column a query gives, NULLs left out.
func column(tx *sql.Tx, q string, args ...any) ([]string, error) {
	rows, err := tx.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if s.Valid {
			out = append(out, s.String)
		}
	}
	return out, rows.Err()
}
