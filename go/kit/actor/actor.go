// Package actor is the keys that name a who: a kind letter and a row id (p1, w2, a17, c3, d4), or a reserved key. Code compares keys,
// never names; a name exists only at the edges (docs/design/people-workspaces.md).
package actor

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Kind is what a key names.
type Kind byte

const (
	None      Kind = 0
	Person    Kind = 'p'
	Workspace Kind = 'w'
	Agent     Kind = 'a'
	Account   Kind = 'c'
	Device    Kind = 'd'
	// Reserved is the daemon's own sender (Sys) or the owner of a task that watches outside state (Gate).
	Reserved Kind = '-'
)

// The reserved keys: no row has them.
const (
	Sys  = "sys"
	Gate = "gate"
)

// OwnerKey is the owner's key where the daemon names the person with no caller in hand: a request to them, their card, an event they
// cause. Personal mode has one person, whom the first start makes p1 (EnsureOwner); phase 4 replaces each use with the agent's owner or
// the task's person.
const OwnerKey = "p1"

// Key is the key of row id of kind k.
func Key(k Kind, id int64) string { return string(rune(k)) + strconv.FormatInt(id, 10) }

// Parse is the kind and row id of key; ok is false for anything that is not a key. A reserved key parses with id 0.
func Parse(key string) (k Kind, id int64, ok bool) {
	if key == Sys || key == Gate {
		return Reserved, 0, true
	}
	if len(key) < 2 {
		return None, 0, false
	}
	switch k = Kind(key[0]); k {
	case Person, Workspace, Agent, Account, Device:
	default:
		return None, 0, false
	}
	id, err := strconv.ParseInt(key[1:], 10, 64)
	if err != nil || id <= 0 || Key(k, id) != key { // only the one spelling: p01 and p+1 are not p1
		return None, 0, false
	}
	return k, id, true
}

// KindOf is the kind key names, None when it is not a key.
func KindOf(key string) Kind {
	k, _, _ := Parse(key)
	return k
}

// IsPerson is whether key names a person.
func IsPerson(key string) bool { return KindOf(key) == Person }

// Q is what a lookup runs on: a *sql.DB or a *sql.Tx.
type Q interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// ErrNoOwner is Owner on a database whose first start has not run.
var ErrNoOwner = errors.New("no owner: the daemon has not started on this home yet")

// Owner is the key of the person who owns this install: the first person with role owner.
func Owner(q Q) (string, error) {
	var id int64
	err := q.QueryRow("SELECT id FROM people WHERE role = 'owner' ORDER BY id LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoOwner
	}
	if err != nil {
		return "", err
	}
	return Key(Person, id), nil
}

// EnsureOwner is the first start: when no person is the owner, it makes one with no handle, named by displayName, which is asked only
// then. It answers the owner's key either way.
func EnsureOwner(q Q, displayName func() string) (string, error) {
	if k, err := Owner(q); !errors.Is(err, ErrNoOwner) {
		return k, err
	}
	name := displayName()
	if name == "" {
		return "", fmt.Errorf("the owner needs a display name")
	}
	// One statement, so two processes starting at once make one owner.
	if _, err := q.Exec("INSERT INTO people(display_name, role, created_at) SELECT ?, 'owner', ? WHERE NOT EXISTS (SELECT 1 FROM people WHERE role = 'owner')",
		name, time.Now().Unix()); err != nil {
		return "", err
	}
	return Owner(q)
}
