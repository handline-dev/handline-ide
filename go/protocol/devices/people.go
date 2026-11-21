package devices

import (
	"database/sql"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// Roles are the roles a person has: the owner decides (trust, settings, people, gates), a member gives work, comments, chats and lends.
var Roles = map[string]bool{"owner": true, "member": true}

// RegisterPeople adds the owner's people methods: invite (the person; device.pair with person makes their link), a role, and remove. The
// people topic tells each change from the table itself.
func RegisterPeople(s *rpc.Server, m *Manager) {
	s.Register("person.invite", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		name, _ := p["display_name"].(string)
		if name = strings.TrimSpace(name); name == "" {
			return nil, rpc.BadRequestError("display_name is the person's name", "display_name", rpc.InvalidParams)
		}
		role := "member"
		if v, has := p["role"]; has {
			if role, _ = v.(string); !Roles[role] {
				return nil, rpc.BadRequestError("role is owner or member", "role", rpc.InvalidParams)
			}
		}
		return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			r, err := tx.Exec("INSERT INTO people(display_name, role, created_at) VALUES(?, ?, ?)", name, role, time.Now().Unix())
			if err != nil {
				return nil, err
			}
			id, err := r.LastInsertId()
			if err != nil {
				return nil, err
			}
			return map[string]any{"accepted": true, "person": actor.Key(actor.Person, id)}, nil
		})
	}})
	s.Register("person.set_role", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		id, err := personParam(p)
		if err != nil {
			return nil, err
		}
		role, _ := p["role"].(string)
		if !Roles[role] {
			return nil, rpc.BadRequestError("role is owner or member", "role", rpc.InvalidParams)
		}
		if err := founderStays(id); err != nil {
			return nil, err
		}
		res, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			was, err := current(tx, id)
			if err != nil {
				return nil, err
			}
			if was == "owner" && role != "owner" {
				if err := notLastOwner(tx, id); err != nil {
					return nil, err
				}
			}
			if _, err := tx.Exec("UPDATE people SET role = ? WHERE id = ?", role, id); err != nil {
				return nil, err
			}
			return map[string]any{"accepted": true}, nil
		})
		if err == nil && m != nil { // a connection holds the role it said hello with: it logs in again under the new one
			m.CloseAllOf(actor.Key(actor.Person, id))
		}
		return res, err
	}})
	s.Register("person.remove", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		id, err := personParam(p)
		if err != nil {
			return nil, err
		}
		if err := founderStays(id); err != nil {
			return nil, err
		}
		// The devices go before the row says removed, outside the write: a retry that finds the write kept answers without running this,
		// and a revoked token cannot log in meanwhile. Redeem refuses a code of a removed person.
		if m != nil {
			if err := m.RevokeAllOf(actor.Key(actor.Person, id)); err != nil {
				return nil, err
			}
		}
		return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			was, err := current(tx, id)
			if err != nil {
				return nil, err
			}
			if was == "owner" {
				if err := notLastOwner(tx, id); err != nil {
					return nil, err
				}
			}
			if _, err := tx.Exec("UPDATE people SET removed_at = ? WHERE id = ?", time.Now().Unix(), id); err != nil {
				return nil, err
			}
			return map[string]any{"accepted": true}, nil
		})
	}})
}

// founderStays refuses a change to the founder's person (actor.OwnerKey): the daemon names them as the owner wherever it has no caller.
func founderStays(id int64) error {
	if actor.Key(actor.Person, id) == actor.OwnerKey {
		return rpc.Refused("the founder stays an owner", "last_owner", "")
	}
	return nil
}

func personParam(p map[string]any) (int64, error) {
	key, _ := p["person"].(string)
	k, id, ok := actor.Parse(key)
	if !ok || k != actor.Person {
		return 0, rpc.BadRequestError("person is a person's key (p<n>)", "person", rpc.InvalidParams)
	}
	return id, nil
}

// current is the role of the person who has not been removed.
func current(tx *sql.Tx, id int64) (string, error) {
	var role string
	err := tx.QueryRow("SELECT role FROM people WHERE id = ? AND removed_at IS NULL", id).Scan(&role)
	if err == sql.ErrNoRows {
		return "", rpc.NotFoundError("no such person: " + actor.Key(actor.Person, id))
	}
	return role, err
}

func notLastOwner(tx *sql.Tx, id int64) error {
	var others int
	if err := tx.QueryRow("SELECT COUNT(*) FROM people WHERE role = 'owner' AND removed_at IS NULL AND id != ?", id).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return rpc.Refused("the last owner stays an owner: make another person owner first", "last_owner", "")
	}
	return nil
}
