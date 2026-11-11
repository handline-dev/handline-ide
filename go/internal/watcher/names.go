package watcher

import (
	"database/sql"
	"errors"

	"github.com/handline/handline/go/data/stream"
)

// Names is a topic whose list a client reads again on any change (people.list, workspaces.list): a write to its table is one event with
// no data. Its first look says nothing.
type Names struct {
	Table, Topic string
	seen         *int64
}

// The two Names topics.
func People() *Names     { return &Names{Table: "people", Topic: "people"} }
func Workspaces() *Names { return &Names{Table: "workspaces", Topic: "workspaces"} }

func (n *Names) Name() string     { return n.Topic }
func (n *Names) Tables() []string { return []string{n.Table} }

func (n *Names) Scan(q *sql.DB, emit Emit) error {
	var c int64
	if err := q.QueryRow("SELECT n FROM table_changes WHERE name = ?", n.Table).Scan(&c); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	before := n.seen
	n.seen = &c
	if before == nil || *before == c {
		return nil
	}
	_, err := emit(stream.Entry{Topic: n.Topic, Type: n.Topic + ".changed", Data: map[string]any{}})
	return err
}
