package paths

import (
	"database/sql"
	"errors"
	"path/filepath"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/store"
)

// Folders are named by key (docs/design/people-workspaces.md, Disk), so a rename moves nothing. Until the who columns hold keys
// (base-v2 1b), callers still name an agent by its old dotted name, and the folder helpers look it up here.

// unregistered holds the folders of a name no agent has: nothing lists it, and no key can be it.
const unregistered = ".unregistered"

// AgentKey is who's key: who itself when it is an agent key, else the key of the agent whose old dotted name it is, "" when none is.
func AgentKey(home, who string) string {
	if actor.KindOf(who) == actor.Agent {
		return who
	}
	d, err := store.SharedIfThere(DBFile(home))
	if d == nil || err != nil {
		return ""
	}
	k, _ := AgentKeyIn(d, who)
	return k
}

// AgentKeyIn is AgentKey read through q, for a caller inside a transaction: "" and no error when no agent has the name.
func AgentKeyIn(q interface {
	QueryRow(string, ...any) *sql.Row
}, legacy string) (string, error) {
	var id int64
	err := q.QueryRow(`SELECT a.id FROM agents a JOIN workspaces w ON w.id = a.workspace
		WHERE CASE WHEN w.lead = a.id THEN w.name ELSE w.name || '.' || a.name END = ?`, legacy).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return actor.Key(actor.Agent, id), nil
}

// AgentDir is the agent's folder, agents/<key>; who is its key or its old dotted name.
func AgentDir(home, who string) string {
	if k := AgentKey(home, who); k != "" {
		return filepath.Join(home, "agents", k)
	}
	return filepath.Join(home, "agents", unregistered, who)
}
