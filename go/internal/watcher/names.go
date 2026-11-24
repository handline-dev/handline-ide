package watcher

import (
	"database/sql"
	"errors"

	"github.com/handline/handline/go/data/stream"
)

// Names is a topic whose list a client reads again on any change (people.list, workspaces.list): a write to one of its tables is one
// event with no data. Its first look says nothing.
type Names struct {
	Topic  string
	tables []string
	seen   *int64
}

// The Names topics.
func People() *Names     { return &Names{Topic: "people", tables: []string{"people"}} }
func Workspaces() *Names { return &Names{Topic: "workspaces", tables: []string{"workspaces"}} }

// Nodes is node.status's topic: its devices, and agents' placement (pin, vendor).
func Nodes() *Names { return &Names{Topic: "nodes", tables: []string{"nodes", "agents"}} }

func (n *Names) Name() string     { return n.Topic }
func (n *Names) Tables() []string { return n.tables }

func (n *Names) Scan(q *sql.DB, emit Emit) error {
	var c int64
	for _, t := range n.tables {
		var one int64
		if err := q.QueryRow("SELECT n FROM table_changes WHERE name = ?", t).Scan(&one); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		c += one
	}
	before := n.seen
	n.seen = &c
	if before == nil || *before == c {
		return nil
	}
	_, err := emit(stream.Entry{Topic: n.Topic, Type: n.Topic + ".changed", Data: map[string]any{}})
	return err
}
