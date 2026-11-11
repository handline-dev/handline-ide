// Package addr turns the words an agent or a person types into keys, and keys back into the words a reader would use
// (docs/design/people-workspaces.md, Addresses and Mentions). It is the only place a who word becomes a key.
package addr

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
)

// Q is what the lookups read: a *sql.DB or a *sql.Tx.
type Q interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// Who is the one a word is resolved for, or a text written out for: its key and the workspace whose agent names it may use (0 for
// none, as for a person typing outside every project folder).
type Who struct {
	Key       string
	Workspace int64
}

// Of is who key is: an agent's own workspace, or for a person the workspace whose project folder holds cwd (none when cwd is "" or
// in no workspace's folder).
func Of(q Q, key, cwd string) (Who, error) {
	w := Who{Key: key}
	switch k, id, _ := actor.Parse(key); k {
	case actor.Agent:
		err := q.QueryRow("SELECT workspace FROM agents WHERE id = ?", id).Scan(&w.Workspace)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		return w, err
	case actor.Person:
		if cwd == "" {
			return w, nil
		}
		rows, err := q.Query("SELECT id, folder FROM workspaces")
		if err != nil {
			return w, err
		}
		defer rows.Close()
		best := -1
		for rows.Next() {
			var id int64
			var folder string
			if err := rows.Scan(&id, &folder); err != nil {
				return w, err
			}
			// The deepest folder wins, so a project inside another resolves to its own workspace.
			if within(cwd, folder) && len(folder) > best {
				best, w.Workspace = len(folder), id
			}
		}
		return w, rows.Err()
	}
	return w, nil
}

func within(dir, folder string) bool {
	if folder == "" {
		return false
	}
	rel, err := filepath.Rel(evaluated(folder), evaluated(dir))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// evaluated is p with its symlinks followed (/var is /private/var on macOS), or cleaned when it cannot be.
func evaluated(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// Refusal is a word that names no one the caller may reach: Msg lists the names it may use.
type Refusal struct{ Msg string }

func (r *Refusal) Error() string { return r.Msg }

// Resolve is the key word names for c, tried in this order: human (c's owner), a key that names a row, an agent of c's workspace, a
// workspace (its lead), a handle. Until the cutover imports every agent under its new name (base-v2 1f), an agent's old dotted name
// and founder (for human) still resolve last. Anything else is a *Refusal.
func Resolve(q Q, c Who, word string) (string, error) {
	if word == "human" {
		return ownerOf(q, c)
	}
	if k, id, ok := actor.Parse(word); ok {
		if there, err := exists(q, k, id); there || err != nil {
			return word, err
		}
	}
	try := []struct {
		query string
		args  []any
		kind  actor.Kind
	}{
		{"SELECT id FROM agents WHERE workspace = ? AND name = ?", []any{c.Workspace, word}, actor.Agent},
		{"SELECT lead FROM workspaces WHERE name = ? AND lead IS NOT NULL", []any{word}, actor.Agent},
		{"SELECT id FROM people WHERE handle = ?", []any{word}, actor.Person},
		{`SELECT a.id FROM agents a JOIN workspaces w ON w.id = a.workspace WHERE ` + paths.LegacyNameSQL + ` = ?`, []any{word}, actor.Agent},
	}
	for _, t := range try {
		var id int64
		err := q.QueryRow(t.query, t.args...).Scan(&id)
		if err == nil {
			return actor.Key(t.kind, id), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	if word == "founder" {
		return ownerOf(q, c)
	}
	return "", refusal(q, c, word)
}

// ownerOf is the person human names for c: an agent's owner, a person themselves.
func ownerOf(q Q, c Who) (string, error) {
	if k, id, _ := actor.Parse(c.Key); k == actor.Agent {
		var owner string
		err := q.QueryRow("SELECT owner FROM agents WHERE id = ?", id).Scan(&owner)
		if err == nil && owner != "" {
			return owner, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	} else if k == actor.Person {
		return c.Key, nil
	}
	var id int64
	err := q.QueryRow("SELECT id FROM people WHERE role = 'owner' ORDER BY id LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", actor.ErrNoOwner
	}
	return actor.Key(actor.Person, id), err
}

func exists(q Q, k actor.Kind, id int64) (bool, error) {
	table := map[actor.Kind]string{actor.Person: "people", actor.Workspace: "workspaces", actor.Agent: "agents", actor.Account: "accounts"}[k]
	switch {
	case k == actor.Reserved:
		return true, nil
	case k == actor.Device:
		var n int
		err := q.QueryRow("SELECT COUNT(*) FROM devices WHERE id = ?", actor.Key(k, id)).Scan(&n)
		return n > 0, err
	case table == "":
		return false, nil
	}
	var n int
	err := q.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE id = ?", id).Scan(&n)
	return n > 0, err
}

// refusal says word names no one, and which words c may use instead.
func refusal(q Q, c Who, word string) error {
	var agents, workspaces, handles []string
	read := func(into *[]string, query string, args ...any) error {
		rows, err := q.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return err
			}
			*into = append(*into, s)
		}
		return rows.Err()
	}
	if err := errors.Join(
		read(&agents, "SELECT name FROM agents WHERE workspace = ? ORDER BY name", c.Workspace),
		read(&workspaces, "SELECT name FROM workspaces WHERE id != ? ORDER BY name", c.Workspace),
		read(&handles, "SELECT handle FROM people WHERE handle IS NOT NULL AND handle != '' ORDER BY handle"),
	); err != nil {
		return err
	}
	parts := []string{"human"}
	if len(agents) > 0 {
		parts = append(parts, "an agent of your workspace ("+strings.Join(agents, ", ")+")")
	}
	if len(workspaces) > 0 {
		parts = append(parts, "a workspace, whose lead receives it ("+strings.Join(workspaces, ", ")+")")
	}
	if len(handles) > 0 {
		parts = append(parts, "a person ("+strings.Join(handles, ", ")+")")
	}
	return &Refusal{fmt.Sprintf("%q names no one you can reach: use %s", word, strings.Join(parts, ", or "))}
}

// Name is the word r would use for key: an agent of r's workspace by its name, another workspace's lead by that workspace's name,
// another workspace's agent as "name (workspace)" (display only: it does not resolve), r's owner or r itself as a person as human, any
// other person by handle or display name. A key that names no row is written as it is.
func Name(q Q, r Who, key string) string {
	k, id, ok := actor.Parse(key)
	if !ok {
		return key
	}
	var name string
	var err error
	switch k {
	case actor.Reserved:
		if key == actor.Sys {
			return "task-board"
		}
		return key
	case actor.Agent:
		var ws, lead int64
		var wsName string
		err = q.QueryRow(`SELECT a.name, a.workspace, w.name, COALESCE(w.lead, 0) FROM agents a JOIN workspaces w ON w.id = a.workspace
			WHERE a.id = ?`, id).Scan(&name, &ws, &wsName, &lead)
		switch {
		case err != nil:
		case ws == r.Workspace:
		case lead == id:
			name = wsName
		default:
			name += " (" + wsName + ")"
		}
	case actor.Person:
		if owner, _ := ownerOf(q, r); owner == key {
			return "human"
		}
		err = q.QueryRow("SELECT COALESCE(NULLIF(handle, ''), display_name) FROM people WHERE id = ?", id).Scan(&name)
	case actor.Workspace:
		err = q.QueryRow("SELECT name FROM workspaces WHERE id = ?", id).Scan(&name)
	case actor.Account:
		err = q.QueryRow("SELECT COALESCE(NULLIF(label, ''), name) FROM accounts WHERE id = ?", id).Scan(&name)
	case actor.Device:
		err = q.QueryRow("SELECT name FROM devices WHERE id = ?", key).Scan(&name)
	}
	if err != nil || name == "" {
		return key
	}
	return name
}

// Address is the word the reader whose key is reader can type for key: its Name when that resolves back to key, else the key itself (an
// agent of another workspace, the daemon), which resolves too.
func Address(q Q, reader, key string) string {
	r, err := Of(q, reader, "")
	if err != nil {
		return key
	}
	if n := Name(q, r, key); n != key {
		if k, err := Resolve(q, r, n); err == nil && k == key {
			return n
		}
	}
	return key
}

// Ref is key as a token in text the daemon writes for more than one reader: each reader's copy writes it out as @ and its name
// (Render), and a rename changes no stored text. A command a reader types names the who with Address instead.
func Ref(key string) string {
	if _, _, ok := actor.Parse(key); !ok {
		return key // a word that is no key (an old name, a phrase) stays as it is
	}
	return "<@" + key + ">"
}

// For is Name for the reader whose key is reader, in its own workspace: for a sentence the daemon writes to one reader.
func For(q Q, reader, key string) string {
	r, err := Of(q, reader, "")
	if err != nil {
		return key
	}
	return Name(q, r, key)
}

var (
	refRe = regexp.MustCompile(`<@([a-z]+[0-9]*)>`)
	// mentionRe is an @word where a mention may start: not inside a word or an address like a@b.
	mentionRe = regexp.MustCompile(`(^|[^A-Za-z0-9_.@<-])@([A-Za-z0-9][A-Za-z0-9_.-]*)`)
)

// Render is text for r: each <@key> becomes @ and the name r would use for that key.
func Render(q Q, r Who, text string) string {
	if !strings.Contains(text, "<@") {
		return text
	}
	return refRe.ReplaceAllStringFunc(text, func(m string) string {
		key := refRe.FindStringSubmatch(m)[1]
		if _, _, ok := actor.Parse(key); !ok {
			return m
		}
		return "@" + Name(q, r, key)
	})
}

// Mentions is text as it is stored: each @word that resolves for writer becomes <@key>. An @word that does not resolve stays text.
func Mentions(q Q, writer Who, text string) (string, error) {
	var failed error
	out := mentionRe.ReplaceAllStringFunc(text, func(m string) string {
		s := mentionRe.FindStringSubmatch(m)
		word := strings.TrimRight(s[2], ".-") // a sentence's full stop is not part of the name
		key, err := Resolve(q, writer, word)
		var refused *Refusal
		if errors.As(err, &refused) || key == "" {
			return m
		}
		if err != nil {
			failed = err
			return m
		}
		return s[1] + Ref(key) + s[2][len(word):]
	})
	return out, failed
}

// Refs is each key text names with <@key>, once, in order.
func Refs(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range refRe.FindAllStringSubmatch(text, -1) {
		if _, _, ok := actor.Parse(m[1]); ok && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// Lead is the first token of text when it is <@p…>: the comment is a note to that person and moves no turn.
func Lead(text string) string {
	t := strings.TrimLeft(text, " \t\r\n")
	m := refRe.FindStringSubmatchIndex(t)
	if m == nil || m[0] != 0 {
		return ""
	}
	return t[m[2]:m[3]]
}
