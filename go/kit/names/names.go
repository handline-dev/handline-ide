// Package names holds the rules for the words people choose: workspace names, agent names and handles. Collisions are refused when
// something is created or renamed, so any word points to one thing (docs/design/people-workspaces.md, Names).
package names

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
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

// Plain is the names a new agent gets when none is typed, tried in order: short, readable, none of them a key's shape.
var Plain = []string{"ada", "bea", "cal", "dot", "eli", "fay", "gus", "ivy", "jo", "kit", "lou", "mia", "ned", "ola", "pip",
	"quin", "rex", "sam", "tess", "uma", "val", "wes", "xia", "yan", "zoe"}

// Free is the first word Check passes for kind: each of words in order, then the first word with -2, -3 and so on.
func Free(q Q, kind Kind, workspace int64, words ...string) (string, error) {
	for _, w := range words {
		if ok, err := passes(q, kind, w, workspace); ok || err != nil {
			return w, err
		}
	}
	base := words[0]
	if r := []rune(base); len(r) > 28 {
		base = string(r[:28]) // room for a suffix up to -999 within the 32 Check allows
	}
	for i := 2; i < 1000; i++ {
		w := fmt.Sprintf("%s-%d", base, i)
		if ok, err := passes(q, kind, w, workspace); ok || err != nil {
			return w, err
		}
	}
	return "", refuse("no free %s from %q", kind, words[0])
}

// passes is whether Check takes word, its refusal read as false and any other error returned.
func passes(q Q, kind Kind, word string, workspace int64) (bool, error) {
	err := Check(q, kind, word, workspace)
	if _, refused := errors.AsType[*Refusal](err); refused {
		return false, nil
	}
	return err == nil, err
}

var notShape = regexp.MustCompile(`[^a-z0-9]+`)

// From is word made into a name's shape (a folder's or repo's name as a workspace name): lowercase, each run of other characters a
// single -, starting with a letter, at most 32 long; "" when nothing of it is left.
func From(word string) string {
	s := strings.Trim(notShape.ReplaceAllString(strings.ToLower(word), "-"), "-")
	s = strings.TrimLeft(s, "0123456789-")
	if len(s) > 32 {
		s = strings.TrimRight(s[:32], "-")
	}
	return s
}
