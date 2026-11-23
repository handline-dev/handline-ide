package reads

import (
	"database/sql"
	"github.com/handline/handline/go/kit/version"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/settings"
)

// nodeStatus is node.status: the machines of this master on run or pause, its own first. This daemon is the master, so its own row is
// master, self and online; another is online while its heartbeat comes.
func (e *Env) nodeStatus(_ *rpc.Conn, _ map[string]any) (any, error) {
	self := joined.NodeIDOf(e.Home)
	rs, err := e.DB.Query(`SELECT id, person, name, joined_at, last_seen, master_number, lending, device, memory_max_gb, door,
		COALESCE(door_fp, '') FROM nodes WHERE lending IN ('run', 'pause') ORDER BY id != ?, joined_at, id`, self)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	nodes := []any{}
	number := int64(1)
	for rs.Next() {
		var id, name, lending string
		var person, seen, memMax sql.NullInt64
		var device, door sql.NullString
		var fp string
		var joined, master int64
		if err := rs.Scan(&id, &person, &name, &joined, &seen, &master, &lending, &device, &memMax, &door, &fp); err != nil {
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
			"lending": lending, "device": nil, "memory_max_gb": nil, "load": nil, "battery": nil, "version": nil, "door": nil,
			"set_aside": nil, "app": nil}
		if id == self {
			row["version"] = version.Sha
		}
		if door.String != "" {
			row["door"] = map[string]any{"addr": door.String, "fp": fp}
		}
		if id != self && e.NodeSetAside != nil {
			row["set_aside"] = e.NodeSetAside(id)
		}
		if id != self && e.NodeApp != nil {
			row["app"] = e.NodeApp(id)
		}
		if device.Valid && id != self { // the master's own device is its own, which no one paired
			row["device"] = device.String
		}
		if memMax.Valid {
			row["memory_max_gb"] = memMax.Int64
		}
		if id != self && e.NodeHeard != nil {
			if load, battery, v, ok := e.NodeHeard(id); ok {
				row["load"], row["battery"], row["version"] = load, battery, v
			}
		}
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
	return map[string]any{"master": map[string]any{"id": self, "number": number}, "nodes": nodes,
		"cluster": map[string]any{"name": ClusterName(e.DB), "machines": len(nodes)}}, rs.Err()
}

// ClusterName is the cluster's name (cluster_name, the board's); "" with no board or before the daemon's start gave it one.
func ClusterName(db *sql.DB) string {
	var name sql.NullString
	if db == nil {
		return ""
	}
	db.QueryRow("SELECT value FROM meta WHERE name = ?", settings.ClusterNameKey).Scan(&name)
	return name.String
}

// Machines is how many machines the cluster has: the master and those lent on run or pause, as node.status lists them.
func Machines(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM nodes WHERE lending IN ('run', 'pause')").Scan(&n)
	return n, err
}
