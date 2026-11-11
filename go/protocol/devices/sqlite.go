package devices

import (
	"database/sql"

	"github.com/handline/handline/go/kit/actor"
)

// SQLStore keeps devices in the daemon's database, in the devices table. Revoking a device deletes its row, and its push token with it.
type SQLStore struct{ DB *sql.DB }

// Add stores a paired device as the owner's: in personal mode the owner is the only person who pairs one.
func (s SQLStore) Add(d Device, tokenHash string) error {
	person, err := actor.Owner(s.DB)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO devices(id, person, name, token_hash, created, last_seen, address, kind) VALUES(?,?,?,?,?,NULL,NULLIF(?,''),NULLIF(?,''))",
		d.ID, person, d.Name, tokenHash, d.Created, d.Address, d.Kind)
	return err
}

const columns = "id, name, created, last_seen, push_platform, push_token, push_sandbox, COALESCE(address, ''), COALESCE(kind, ''), COALESCE(client, ''), person"

type scanner interface{ Scan(dest ...any) error }

func scan(r scanner) (Device, error) {
	var d Device
	var seen, sandbox sql.NullInt64
	var platform, token sql.NullString
	if err := r.Scan(&d.ID, &d.Name, &d.Created, &seen, &platform, &token, &sandbox, &d.Address, &d.Kind, &d.Client, &d.Person); err != nil {
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
	d, err := scan(s.DB.QueryRow("SELECT "+columns+" FROM devices WHERE token_hash = ?", hash))
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
	rows, err := s.DB.Query("SELECT " + columns + " FROM devices ORDER BY created, id")
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

func (s SQLStore) Rename(id, name string) (bool, error) {
	r, err := s.DB.Exec("UPDATE devices SET name = ? WHERE id = ?", name, id)
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
