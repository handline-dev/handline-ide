package devices

import (
	"database/sql"
	"fmt"

	"github.com/handline/handline/go/kit/actor"
)

// SQLStore keeps devices in the daemon's database, in the devices table. Revoking a device deletes its row, and its push token with it.
type SQLStore struct{ DB *sql.DB }

// Add stores a paired device as d.Person's, the owner's when it names none. A device with a node makes that machine's node row, on Off,
// in the same write; a node id the master already has is ErrAlreadyJoined.
func (s SQLStore) Add(d Device, tokenHash string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	person := d.Person
	if person == "" {
		if person, err = actor.Owner(tx); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT INTO devices(id, person, name, token_hash, created, last_seen, address, kind) VALUES(?,?,?,?,?,NULL,NULLIF(?,''),NULLIF(?,''))",
		d.ID, person, d.Name, tokenHash, d.Created, d.Address, d.Kind); err != nil {
		return err
	}
	if d.Node != "" {
		var has int
		if err := tx.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", d.Node).Scan(&has); err != nil {
			return err
		}
		if has > 0 {
			return ErrAlreadyJoined
		}
		k, pid, _ := actor.Parse(person)
		if k != actor.Person {
			return fmt.Errorf("the device's person %q is no person's key", person)
		}
		if _, err := tx.Exec("INSERT INTO nodes(id, person, name, joined_at, device, lending) VALUES(?, ?, ?, ?, ?, 'off')",
			d.Node, pid, d.Name, d.Created, d.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// columns is a device with its node's id and lending, from devices LEFT JOIN nodes.
const columns = "d.id, d.name, d.created, d.last_seen, d.push_platform, d.push_token, d.push_sandbox, COALESCE(d.address, ''), " +
	"COALESCE(d.kind, ''), COALESCE(d.client, ''), d.person, COALESCE(n.id, ''), COALESCE(n.lending, '')"

const from = " FROM devices d LEFT JOIN nodes n ON n.device = d.id"

type scanner interface{ Scan(dest ...any) error }

func scan(r scanner) (Device, error) {
	var d Device
	var seen, sandbox sql.NullInt64
	var platform, token sql.NullString
	if err := r.Scan(&d.ID, &d.Name, &d.Created, &seen, &platform, &token, &sandbox, &d.Address, &d.Kind, &d.Client, &d.Person, &d.Node, &d.Lending); err != nil {
		return Device{}, err
	}
	if seen.Valid {
		d.LastSeen = &seen.Int64
	}
	if token.String != "" {
		d.Push = &Push{Platform: platform.String, Token: token.String, Sandbox: sandbox.Int64 != 0}
	}
	return d, nil
}

func (s SQLStore) ByTokenHash(hash string) (Device, bool, error) {
	d, err := scan(s.DB.QueryRow("SELECT "+columns+from+" WHERE d.token_hash = ?", hash))
	if err == sql.ErrNoRows {
		return Device{}, false, nil
	}
	return d, err == nil, err
}

func (s SQLStore) Delete(id string) (bool, error) {
	r, err := s.DB.Exec("DELETE FROM devices WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n > 0, err
}

func (s SQLStore) List() ([]Device, error) {
	rows, err := s.DB.Query("SELECT " + columns + from + " ORDER BY d.created, d.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s SQLStore) Touch(id string, ts int64) error {
	_, err := s.DB.Exec("UPDATE devices SET last_seen = ? WHERE id = ?", ts, id)
	return err
}

func (s SQLStore) SetClient(id, client string) error {
	_, err := s.DB.Exec("UPDATE devices SET client = ? WHERE id = ?", client, id)
	return err
}

// SetLending sets the run, pause or off of the device's node row while the device is person's; false when it is not, or has none.
func (s SQLStore) SetLending(id, lending, person string) (bool, error) {
	r, err := s.DB.Exec("UPDATE nodes SET lending = ? WHERE device = ? AND device IN (SELECT id FROM devices WHERE person = ?)", lending, id, person)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n > 0, err
}

func (s SQLStore) SetPush(id string, p Push) (bool, error) {
	r, err := s.DB.Exec("UPDATE devices SET push_platform = ?, push_token = ?, push_sandbox = ? WHERE id = ?", p.Platform, p.Token, p.Sandbox, id)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n > 0, err
}

func (s SQLStore) DropPush(id, token string) error {
	_, err := s.DB.Exec("UPDATE devices SET push_platform = NULL, push_token = NULL, push_sandbox = NULL WHERE id = ? AND push_token = ?", id, token)
	return err
}
