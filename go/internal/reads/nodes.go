package reads

import (
	"database/sql"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/protocol/rpc"
)

// nodeStatus is node.status: the machines of this master on run or pause, its own first. This daemon is the master, so its own row is
// master, self and online; another is online while its heartbeat comes.
func (e *Env) nodeStatus(_ *rpc.Conn, _ map[string]any) (any, error) {
	self := joined.NodeIDOf(e.Home)
	rs, err := e.DB.Query(`SELECT id, person, name, joined_at, last_seen, master_number, lending FROM nodes
		WHERE lending IN ('run', 'pause') ORDER BY id != ?, joined_at, id`, self)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	nodes := []any{}
	number := int64(1)
	for rs.Next() {
		var id, name, lending string
		var person, seen sql.NullInt64
		var joined, master int64
		if err := rs.Scan(&id, &person, &name, &joined, &seen, &master, &lending); err != nil {
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
		row := map[string]any{"id": id, "person": who, "name": name, "joined_at": ts(joined), "last_seen": last,
			"master_number": master, "master": id == self, "self": id == self, "online": id == self || e.NodeOnline != nil && e.NodeOnline(id),
			"lending": lending}
		if id != self && e.NodeCopy != nil {
			row["copy"] = e.NodeCopy(id)
		}
		if id != self && e.NodeLacks != nil {
			lacks := []any{}
			for _, a := range e.listAgents() {
				if why := e.NodeLacks(a, id); why != "" {
					lacks = append(lacks, map[string]any{"agent": a, "why": why})
				}
			}
			row["lacks"] = lacks
		}
		if id != self && e.NodeProjects != nil {
			if ps := e.NodeProjects(id); ps != nil {
				row["projects"] = ps
			}
		}
		nodes = append(nodes, row)
	}
	return map[string]any{"master": map[string]any{"id": self, "number": number}, "nodes": nodes}, rs.Err()
}
