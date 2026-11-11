// Package names holds the rules for the words people choose: workspace names, agent names and handles. Collisions are refused when
// something is created or renamed, so any word points to one thing (docs/design/people-workspaces.md, Names).
package names

import (
	"database/sql"
	"fmt"
	"regexp"
)

// Kind is what a word would name.
type Kind int

const (
	Workspace Kind = iota
	Agent
	Handle
)

func (k Kind) String() string {
	switch k {
	case Workspace:
		return "workspace name"
	case Agent:
		return "agent name"
	}
	return "handle"
}

// Lead is the name a new workspace's first agent gets.
const Lead = "lead"

// Reserved are the words no workspace, agent or person may take.
var Reserved = []string{"human", "me", "all", "everyone", "sys", "gate"}

var (
	shape = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	// keyShape is a word that would read as a key (a17, p2) in logs and hl output.
	keyShape = regexp.MustCompile(`^[pwacd][0-9]+$`)
)

// Q is what the checks read: a *sql.DB or a *sql.Tx.
type Q interface {
	QueryRow(query string, args ...any) *sql.Row
}

// Refusal is why a word was refused, in words the app and hl show as they are.
type Refusal struct{ Msg string }

func (r *Refusal) Error() string { return r.Msg }

func refuse(format string, a ...any) error { return &Refusal{fmt.Sprintf(format, a...)} }

// Check is nil when word may become a new kind name; workspace is the agent's workspace id, used for Agent only. A rename checks the
// new word the same way. Run it and the write in one transaction: no constraint covers the rules across tables.
func Check(q Q, kind Kind, word string, workspace int64) error {
	if !shape.MatchString(word) {
		return refuse("%q is not a %s: use lowercase letters, digits and -, 1 to 32 long, starting with a letter", word, kind)
	}
	if keyShape.MatchString(word) {
		return refuse("%q reads as a key: choose another", word)
	}
	for _, r := range Reserved {
		if word == r {
			return refuse("%q is reserved", word)
		}
	}
	taken := func(query string, args ...any) (bool, error) {
		var n int
		err := q.QueryRow(query, args...).Scan(&n)
		return n > 0, err
	}
	type rule struct {
		what, query string
		args        []any
	}
	rules := []rule{
		{"a workspace", "SELECT COUNT(*) FROM workspaces WHERE name = ?", []any{word}},
		{"a person's handle", "SELECT COUNT(*) FROM people WHERE handle = ?", []any{word}},
	}
	// An agent name is unique within its workspace and may repeat across workspaces, but a workspace name or a handle may equal no
	// agent name anywhere.
	if kind == Agent {
		rules = append(rules, rule{"an agent of this workspace", "SELECT COUNT(*) FROM agents WHERE name = ? AND workspace = ?", []any{word, workspace}})
	} else {
		rules = append(rules, rule{"an agent", "SELECT COUNT(*) FROM agents WHERE name = ?", []any{word}})
	}
	for _, r := range rules {
		hit, err := taken(r.query, r.args...)
		if err != nil {
			return err
		}
		if hit {
			return refuse("%q is already the name of %s", word, r.what)
		}
	}
	return nil
}
