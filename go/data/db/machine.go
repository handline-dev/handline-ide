package db

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MachineIDKey is the meta row the machine's id was kept in before it had its own file; NodeID seeds the file from it.
const MachineIDKey = "machine_id"

// NodeIDFile is where a home keeps its node id: outside the board, so a copy of the board on another machine does not carry it.
func NodeIDFile(home string) string { return filepath.Join(home, "node_id") }

// NodeIDOf is the node id NodeID kept in home, "" before there is one.
func NodeIDOf(home string) string {
	if home == "" {
		return ""
	}
	b, _ := os.ReadFile(NodeIDFile(home))
	return strings.TrimSpace(string(b))
}

// NodeID is this machine's id (Machine.id, nodes.id): made once, at the daemon's start, and kept in NodeIDFile. A home that had one in
// meta keeps it, and the meta row goes, so a copy of the board made later seeds no other machine with it.
func NodeID(home string, q Q) (string, error) {
	if home == "" {
		return "", errors.New("no home to keep the node id in")
	}
	file := NodeIDFile(home)
	if id := NodeIDOf(home); id != "" {
		return id, nil
	} else if _, err := os.Stat(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	id, ok, err := Meta(q, MachineIDKey)
	if err != nil {
		return "", err
	}
	if !ok || id == "" {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		id = hex.EncodeToString(b)
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, file); err != nil {
		os.Remove(tmp)
		return "", err
	}
	_, err = q.Exec("DELETE FROM meta WHERE name = ?", MachineIDKey)
	return id, err
}

// KeepOwnNode writes this machine's row in nodes: its owner (the first owner person), name and last_seen now; joined_at stays what the
// first write gave it.
func KeepOwnNode(q Q, id, name string, now int64) error {
	_, err := q.Exec(`INSERT INTO nodes(id, person, name, joined_at, last_seen)
		VALUES(?, (SELECT MIN(id) FROM people WHERE role = 'owner'), ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, last_seen = excluded.last_seen`,
		id, name, now, now)
	return err
}
