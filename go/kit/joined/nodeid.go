package joined

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

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

// NodeID is this machine's id (Machine.id, nodes.id), made once and kept in NodeIDFile. The first time, seed gives the id a home had
// before ("" or a nil seed for a new one).
func NodeID(home string, seed func() (string, error)) (string, error) {
	if home == "" {
		return "", errors.New("no home to keep the node id in")
	}
	file := NodeIDFile(home)
	if id := NodeIDOf(home); id != "" {
		return id, nil
	} else if _, err := os.Stat(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	var id string
	if seed != nil {
		var err error
		if id, err = seed(); err != nil {
			return "", err
		}
	}
	if id == "" {
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
	return id, nil
}
