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

// Unregistered holds the folders of a name no agent has: nothing lists it, and no key can be it.
const Unregistered = ".unregistered"

// AgentKey is who's key: who itself when it is an agent's key, else the key of the agent whose old dotted name it is. "" when no
// agent is who, or the agents table cannot be read: a caller that must tell the two apart uses AgentKeyIn.
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

// AgentKeyIn is AgentKey read through q, which may be the transaction that changed the agent's row: "" and no error when no agent
// is who.
func AgentKeyIn(q interface {
	QueryRow(string, ...any) *sql.Row
}, who string) (string, error) {
	var id int64
	var err error
	if k, n, ok := actor.Parse(who); ok && k == actor.Agent {
		err = q.QueryRow("SELECT id FROM agents WHERE id = ?", n).Scan(&id)
	} else {
		err = q.QueryRow(`SELECT a.id FROM agents a JOIN workspaces w ON w.id = a.workspace
			WHERE CASE WHEN w.lead = a.id THEN w.name ELSE w.name || '.' || a.name END = ?`, who).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return actor.Key(actor.Agent, id), nil
}

// AgentDir is the agent's folder, agents/<key>; who is its key or its old dotted name. A name not looked up gets
// agents/.unregistered/<name>.
func AgentDir(home, who string) string {
	if k := AgentKey(home, who); k != "" {
		return filepath.Join(home, "agents", k)
	}
	return filepath.Join(home, "agents", Unregistered, who)
}

// CacheDir is a sandboxed agent's own build caches (claude.CacheEnv), caches/<key>, or caches/.unregistered/<name>.
func CacheDir(home, who string) string {
	if k := AgentKey(home, who); k != "" {
		return filepath.Join(home, "caches", k)
	}
	return filepath.Join(home, "caches", Unregistered, who)
}

// AgentKeys is each agent's key by its old dotted name, in one read.
func AgentKeys(home string) (map[string]string, error) {
	d, err := store.SharedIfThere(DBFile(home))
	if d == nil {
		return map[string]string{}, err
	}
	rows, err := d.Query(`SELECT CASE WHEN w.lead = a.id THEN w.name ELSE w.name || '.' || a.name END, a.id
		FROM agents a JOIN workspaces w ON w.id = a.workspace`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var n string
		var id int64
		if err := rows.Scan(&n, &id); err != nil {
			return nil, err
		}
		out[n] = actor.Key(actor.Agent, id)
	}
	return out, rows.Err()
}
