package reads

import (
	"database/sql"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// nodeStatus is node.status: the machines of this master, its own first. This daemon is the master, so its own row is master, self and
// online; no other machine is online until runners report.
func (e *Env) nodeStatus(_ *rpc.Conn, _ map[string]any) (any, error) {
	self := db.NodeIDOf(e.Home)
	rs, err := e.DB.Query("SELECT id, person, name, joined_at, last_seen, master_number FROM nodes ORDER BY id != ?, joined_at, id", self)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	nodes := []any{}
	number := int64(1)
	for rs.Next() {
		var id, name string
		var person, seen sql.NullInt64
		var joined, master int64
		if err := rs.Scan(&id, &person, &name, &joined, &seen, &master); err != nil {
			return nil, err
		}
		var who, last any
		if person.Valid {
			who = actor.Key(actor.Person, person.Int64)
		}
		if seen.Valid {
			last = ts(seen.Int64)
		}
		if id == self {
			number = master
		}
		nodes = append(nodes, map[string]any{"id": id, "person": who, "name": name, "joined_at": ts(joined), "last_seen": last,
			"master_number": master, "master": id == self, "self": id == self, "online": id == self})
	}
	return map[string]any{"master": map[string]any{"id": self, "number": number}, "nodes": nodes}, rs.Err()
}
