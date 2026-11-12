// Package leasestore keeps the held leases (go/scheduler/leases) in the leases table, and finds an agent's workspace for the fair grant.
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

// Load is every stored lease and the highest id the table ever gave.
func (s Store) Load() ([]leases.Lease, int64, error) {
	rs, err := s.DB.Query("SELECT id, kind, agent, workspace, granted_at, pid FROM leases ORDER BY id")
	if err != nil {
		return nil, 0, err
	}
	defer rs.Close()
	var out []leases.Lease
	for rs.Next() {
		var l leases.Lease
		if err := rs.Scan(&l.ID, &l.Kind, &l.Agent, &l.Workspace, &l.Since, &l.PID); err != nil {
			return nil, 0, err
		}
		l.Hold = l.PID == 0
		out = append(out, l)
	}
	if err := rs.Err(); err != nil {
		return nil, 0, err
	}
	var last int64
	err = s.DB.QueryRow("SELECT seq FROM sqlite_sequence WHERE name = 'leases'").Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return out, last, err
}

// Add stores l under its own id.
func (s Store) Add(l leases.Lease) error {
	_, err := s.DB.Exec("INSERT INTO leases (id, kind, agent, workspace, granted_at, pid) VALUES (?, ?, ?, ?, ?, ?)",
		l.ID, l.Kind, l.Agent, l.Workspace, l.Since, l.PID)
	return err
}

// Remove drops lease id.
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
