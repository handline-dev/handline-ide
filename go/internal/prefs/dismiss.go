package prefs

import (
	"database/sql"
	"fmt"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

// RegisterDismiss adds warning.dismiss. sinces is the since of each warning the founder has now, by id, as reads.Env.WarningSinces gives it.
func RegisterDismiss(s *rpc.Server, sinces func() (map[string]string, error)) {
	s.Register("warning.dismiss", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		id, _ := p["id"].(string)
		if id == "" {
			return nil, bad("id is a warning's id (a machine item's id)", "id")
		}
		now, err := sinces()
		if err != nil {
			return nil, fmt.Errorf("warning.dismiss: %w", err)
		}
		return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			if err := Dismiss(tx, id, now); err != nil {
				return nil, err
			}
			return map[string]any{"accepted": true, "said": "warning dismissed"}, nil
		})
	}})
}

// Dismiss keeps the founder's dismissal of warning id with its since, given now, the since of each warning they have, by id.
func Dismiss(tx *sql.Tx, id string, now map[string]string) error {
	since, ok := now[id]
	if !ok {
		nf := rpc.NotFoundError("no warning " + id)
		nf.Data = map[string]any{"what": "warning", "name": id}
		return nf
	}
	return db.SetMeta(tx, db.DismissedPrefix+id, since)
}
