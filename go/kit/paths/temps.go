package paths

import (
	"database/sql"
	"errors"

	"github.com/handline/handline/go/kit/actor"
)

// HiredFrom is the key of the agent the temp key was hired from (agents.hired_from), "" for an agent of its own or no agent.
func HiredFrom(q rowQ, key string) (string, error) {
	from, _, err := seat(q, key)
	return from, err
}

// KnowsAs is whose knowledge agent key works with: the agent a temp was hired from, whose role, lessons and memory folder it shares;
// key itself for an agent of its own.
func KnowsAs(q rowQ, key string) (string, error) {
	from, err := HiredFrom(q, key)
	if from == "" || err != nil {
		return key, err
	}
	return from, nil
}

// Reaches is who a word sent to key reaches: the agent a temp was hired from while the temp sits idle (agents.hired_for unset), else key.
func Reaches(q rowQ, key string) (string, error) {
	from, at, err := seat(q, key)
	if from == "" || at || err != nil {
		return key, err
	}
	return from, nil
}

// seat is key's agents.hired_from, and whether it is at work (agents.hired_for set).
func seat(q rowQ, key string) (from string, atWork bool, err error) {
	k, id, ok := actor.Parse(key)
	if !ok || k != actor.Agent {
		return "", false, nil
	}
	var f sql.NullString
	var task sql.NullInt64
	err = q.QueryRow("SELECT hired_from, hired_for FROM agents WHERE id = ?", id).Scan(&f, &task)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return f.String, task.Valid, err
}
