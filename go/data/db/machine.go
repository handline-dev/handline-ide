package db

import "github.com/handline/handline/go/kit/joined"

// MachineIDKey is the meta row the machine's id was kept in before it had its own file; NodeID seeds the file from it.
const MachineIDKey = "machine_id"

// NodeID is this machine's id (joined.NodeID), made at the daemon's start. A home that had one in meta keeps it, and the meta row
// goes, so a copy of the board made later seeds no other machine with it.
func NodeID(home string, q Q) (string, error) {
	id, err := joined.NodeID(home, func() (string, error) { v, _, err := Meta(q, MachineIDKey); return v, err })
	if err != nil {
		return "", err
	}
	_, err = q.Exec("DELETE FROM meta WHERE name = ?", MachineIDKey)
	return id, err
}

// KeepOwnNode writes this machine's row in nodes: its owner (the first owner person), name and last_seen now; joined_at stays what the
// first write gave it.
func KeepOwnNode(q Q, id, name string, now int64) error {
	_, err := q.Exec(`INSERT INTO nodes(id, person, name, joined_at, last_seen, lending)
		VALUES(?, (SELECT MIN(id) FROM people WHERE role = 'owner'), ?, ?, ?, 'run')
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, last_seen = excluded.last_seen`,
		id, name, now, now)
	return err
}
