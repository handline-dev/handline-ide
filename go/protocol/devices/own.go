package devices

import (
	"database/sql"
	"errors"
	"time"
)

// EnsureOwn is the master's own device, the one its node row names: kept when kept is still its token, else made anew with a new token
// (kind is mac or server, as a join names it). The board keeps only the hash, so the copy carries it to the master that takes over.
func EnsureOwn(db *sql.DB, node, name, kind, kept string, now time.Time) (token, device string, err error) {
	tx, err := db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var dev sql.NullString
	var person sql.NullInt64
	if err := tx.QueryRow("SELECT device, person FROM nodes WHERE id = ?", node).Scan(&dev, &person); err != nil {
		return "", "", err
	}
	if !person.Valid {
		return "", "", errors.New("the master's node row names no person")
	}
	if dev.Valid && kept != "" {
		var n int
		if err := tx.QueryRow("SELECT COUNT(*) FROM devices WHERE id = ? AND token_hash = ?", dev.String, hashOf(kept)).Scan(&n); err != nil {
			return "", "", err
		}
		if n > 0 {
			return kept, dev.String, nil
		}
	}
	token, device = "hld_"+random(32), "dev_"+random(6)
	if _, err := tx.Exec("INSERT INTO devices(id, person, name, token_hash, created, kind) VALUES(?, 'p' || ?, ?, ?, ?, ?)",
		device, person.Int64, name, hashOf(token), now.Unix(), kind); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec("UPDATE nodes SET device = ? WHERE id = ?", device, node); err != nil {
		return "", "", err
	}
	if dev.Valid { // the old one's token is lost: nothing can use it
		if _, err := tx.Exec("DELETE FROM devices WHERE id = ?", dev.String); err != nil {
			return "", "", err
		}
	}
	return token, device, tx.Commit()
}
