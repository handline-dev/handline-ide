package db

import (
	"crypto/rand"
	"encoding/hex"
)

// MachineIDKey is the meta row this machine's id is kept in.
const MachineIDKey = "machine_id"

// MachineID is this machine's id (Machine.id): made on the first ask and kept, so the first one written wins.
func MachineID(q Q) (string, error) {
	if v, ok, err := Meta(q, MachineIDKey); err != nil || ok && v != "" {
		return v, err
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	if _, err := q.Exec("INSERT INTO meta(name, value, updated) VALUES(?,?,?) ON CONFLICT(name) DO NOTHING", MachineIDKey, hex.EncodeToString(b), Now()); err != nil {
		return "", err
	}
	v, _, err := Meta(q, MachineIDKey)
	return v, err
}
