// Package leasestore keeps the held leases and the asks in line (go/scheduler/leases) in the leases table, and finds an agent's
// workspace for the fair grant.
package leasestore

import (
	"database/sql"
	"errors"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/scheduler/leases"
)

// Store is leases.Store on the daemon's database.
type Store struct{ DB *sql.DB }

var _ leases.Store = Store{}

// Load is every stored lease and ask, and the highest id the table ever gave.
func (s Store) Load() ([]leases.Lease, []leases.Ask, int64, error) {
	rs, err := s.DB.Query("SELECT id, kind, agent, workspace, granted_at, pid, task, gate, until, waiting, port FROM leases ORDER BY id")
	if err != nil {
		return nil, nil, 0, err
	}
	defer rs.Close()
	var held []leases.Lease
	var asks []leases.Ask
	for rs.Next() {
		var l leases.Lease
		var gate int64
		var waiting bool
		if err := rs.Scan(&l.ID, &l.Kind, &l.Agent, &l.Workspace, &l.Since, &l.PID, &l.Task, &gate, &l.Until, &waiting, &l.Port); err != nil {
			return nil, nil, 0, err
		}
		if waiting {
			asks = append(asks, leases.Ask{ID: l.ID, Kind: l.Kind, Agent: l.Agent, Workspace: l.Workspace, Task: l.Task, Gate: gate, Since: l.Since})
			continue
		}
		l.Hold = l.PID == 0 && !l.Reserved()
		held = append(held, l)
	}
	if err := rs.Err(); err != nil {
		return nil, nil, 0, err
	}
	var last int64
	err = s.DB.QueryRow("SELECT seq FROM sqlite_sequence WHERE name = 'leases'").Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return held, asks, last, err
}

// Put stores l under its own id, in place of what that id held.
func (s Store) Put(l leases.Lease) error {
	_, err := s.DB.Exec("INSERT OR REPLACE INTO leases (id, kind, agent, workspace, granted_at, pid, task, until, port) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		l.ID, l.Kind, l.Agent, l.Workspace, l.Since, l.PID, l.Task, l.Until, l.Port)
	return err
}

// AddAsk stores an ask in line.
func (s Store) AddAsk(a leases.Ask) error {
	_, err := s.DB.Exec("INSERT INTO leases (id, kind, agent, workspace, granted_at, task, gate, waiting) VALUES (?, ?, ?, ?, ?, ?, ?, 1)",
		a.ID, a.Kind, a.Agent, a.Workspace, a.Since, a.Task, a.Gate)
	return err
}

// Remove drops lease or ask id.
func (s Store) Remove(id int64) error {
	_, err := s.DB.Exec("DELETE FROM leases WHERE id = ?", id)
	return err
}

// WorkspaceOf is the workspace key of the agent whose key is agent, "" when it is not an agent's key or has no row.
func (s Store) WorkspaceOf(agent string) string {
	k, id, ok := actor.Parse(agent)
	if !ok || k != actor.Agent {
		return ""
	}
	var ws int64
	if err := s.DB.QueryRow("SELECT workspace FROM agents WHERE id = ?", id).Scan(&ws); err != nil {
		return ""
	}
	return actor.Key(actor.Workspace, ws)
}
