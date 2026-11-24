package paths

import (
	"database/sql"
	"errors"

	"github.com/handline/handline/go/kit/actor"
)

// HiredFrom is the key of the agent the temp key was hired from (agents.hired_from), "" for an agent of its own or no agent.
func HiredFrom(q rowQ, key string) (string, error) {
	k, id, ok := actor.Parse(key)
	if !ok || k != actor.Agent {
		return "", nil
	}
	var from sql.NullString
	err := q.QueryRow("SELECT hired_from FROM agents WHERE id = ?", id).Scan(&from)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return from.String, err
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
