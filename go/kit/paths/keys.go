package paths

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/store"
)

// Folders are named by key (docs/design/people-workspaces.md, Disk), so a rename moves nothing. A caller may name an agent by any
// word addr.Resolve takes, and the folder helpers look it up here.

// Unregistered holds the folders of a name no agent has: nothing lists it, and no key can be it.
const Unregistered = ".unregistered"

// AgentKey is who's key: who itself when it is an agent's key, else the key of the agent the word names (AgentKeyIn). "" when no
// agent is who, or the agents table cannot be read: a caller that must tell the two apart uses AgentKeyIn.
func AgentKey(home, who string) string { return keyOf(home, who, actor.Agent, AgentKeyIn) }

// rowQ is a database or a transaction, as the key lookups read them.
type rowQ = addr.Q

// keyOf is who itself when it is a key of kind, else the key lookup finds for it in home's database; "" when none is, or the
// database cannot be read.
func keyOf(home, who string, kind actor.Kind, lookup func(rowQ, string) (string, error)) string {
	if actor.KindOf(who) == kind {
		return who
	}
	d, err := store.SharedIfThere(DBFile(home))
	if d == nil || err != nil {
		return ""
	}
	k, _ := lookup(d, who)
	return k
}

// AgentKeyIn is AgentKey read through q, which may be the transaction that changed the agent's row: "" and no error when no agent
// is who. who is any word addr.Resolve takes for no workspace in particular: a key, "workspace/name", a workspace (its lead), or a name
// no other agent has.
func AgentKeyIn(q rowQ, who string) (string, error) {
	k, err := addr.Resolve(q, addr.Who{}, who)
	if _, refused := errors.AsType[*addr.Refusal](err); refused || (err == nil && actor.KindOf(k) != actor.Agent) {
		return "", nil
	}
	return k, err
}

// AgentNamed is the workspace and name of the agent whose key is key, read through q: both "" and no error when no agent has it.
func AgentNamed(q rowQ, key string) (ws, name string, err error) {
	k, id, ok := actor.Parse(key)
	if !ok || k != actor.Agent {
		return "", "", nil
	}
	err = q.QueryRow(`SELECT w.name, a.name FROM agents a JOIN workspaces w ON w.id = a.workspace WHERE a.id = ?`, id).Scan(&ws, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return ws, name, err
}

// AgentDir is the agent's folder, agents/<key>; who is its key or a word that names it. A name not looked up gets
// agents/.unregistered/<name>.
func AgentDir(home, who string) string {
	if k := AgentKey(home, who); k != "" {
		return filepath.Join(home, "agents", k)
	}
	return filepath.Join(home, "agents", Unregistered, who)
}

// CacheDir is a sandboxed agent's own build caches (claude.CacheEnv), caches/<key>, or caches/.unregistered/<name>.
func CacheDir(home, who string) string {
	if k := AgentKey(home, who); k != "" {
		return filepath.Join(home, "caches", k)
	}
	return filepath.Join(home, "caches", Unregistered, who)
}

// AccountKey is who's key: who itself when it is an account's key, else the key of the account named who; "" when none is, or the
// accounts table cannot be read.
func AccountKey(home, who string) string { return keyOf(home, who, actor.Account, AccountKeyIn) }

// AccountKeyIn is the key of the account named name, read through q: "" and no error when none is.
func AccountKeyIn(q rowQ, name string) (string, error) {
	return keyIn(q, "SELECT id FROM accounts WHERE name = ?", actor.Account, name)
}

// keyIn is the kind's key of the row query finds for name: "" and no error when none is.
func keyIn(q rowQ, query string, kind actor.Kind, name string) (string, error) {
	var id int64
	err := q.QueryRow(query, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return actor.Key(kind, id), nil
}

// AccountOfFolder is the name of the account whose folder under accounts/ is folder, "" when none is.
func AccountOfFolder(home, folder string) string {
	d, err := store.SharedIfThere(DBFile(home))
	if folder == "" || d == nil || err != nil {
		return ""
	}
	var id int64
	if k, kid, ok := actor.Parse(folder); ok && k == actor.Account {
		id = kid
	}
	var name string
	if d.QueryRow("SELECT name FROM accounts WHERE folder = ? OR (folder = '' AND id = ?)", folder, id).Scan(&name) != nil {
		return ""
	}
	return name
}

// AccountFolder is the name of the account's folder under accounts/: the one its row records, its key when the row records none (an
// account made before folders were recorded). who is its key or its name. A key no row has is its own folder: a lent machine keeps no
// accounts table. "" for a name no row has.
func AccountFolder(home, who string) string {
	f, _ := accountFolder(home, who)
	return f
}

// accountFolder is AccountFolder, and whether home's accounts table was read.
func accountFolder(home, who string) (f string, read bool) {
	if d, err := store.SharedIfThere(DBFile(home)); d != nil && err == nil {
		f, err = AccountFolderIn(d, who)
		read = err == nil
	}
	if f == "" && actor.KindOf(who) == actor.Account {
		return who, read
	}
	return f, read
}

// AccountFolderIn is AccountFolder read through q: "" and no error when no row is who.
func AccountFolderIn(q rowQ, who string) (string, error) {
	query := "SELECT id, folder FROM accounts WHERE name = ?"
	var arg any = who
	if k, id, ok := actor.Parse(who); ok && k == actor.Account {
		query, arg = "SELECT id, folder FROM accounts WHERE id = ?", id
	}
	var id int64
	var folder string
	err := q.QueryRow(query, arg).Scan(&id, &folder)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return FolderOf(id, folder), nil
}

// FolderOf is the folder of account row id that records folder: the row's key when it records none.
func FolderOf(id int64, folder string) string {
	if folder == "" {
		return actor.Key(actor.Account, id)
	}
	return folder
}

// AccountDir is the account's folder, accounts/<folder> (AccountFolder); who is its key, its name, or, on a home whose accounts table
// cannot be read (a lent machine has none), the folder's own name. A name not looked up gets accounts/.unregistered/<name>.
func AccountDir(home, who string) string {
	f, read := accountFolder(home, who)
	if f != "" {
		return filepath.Join(home, "accounts", f)
	}
	if own := filepath.Join(home, "accounts", who); !read && who != "" && !strings.Contains(who, "/") && !strings.HasPrefix(who, ".") && isDir(own) {
		return own
	}
	return filepath.Join(home, "accounts", Unregistered, who)
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// AccountFolderReadOnly is AccountFolderIn on home's database opened read-only and closed after: for a home this process must neither
// migrate nor write, a live run's real one.
func AccountFolderReadOnly(home, name string) (string, error) {
	d, err := sql.Open("sqlite", "file:"+DBFile(home)+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer d.Close()
	return AccountFolderIn(d, name)
}

// WorkspaceKey is who's key: who itself when it is a workspace's key, else the key of the workspace named who; "" when none is, or
// the workspaces table cannot be read.
func WorkspaceKey(home, who string) string { return keyOf(home, who, actor.Workspace, WorkspaceKeyIn) }

// WorkspaceKeyIn is the key of the workspace named name, read through q: "" and no error when none is.
func WorkspaceKeyIn(q rowQ, name string) (string, error) {
	return keyIn(q, "SELECT id FROM workspaces WHERE name = ?", actor.Workspace, name)
}

// WhoKey is the key a who param's word stands for: human is the owner, a key is itself, a word that names an agent its key. A word
// none of these is, or a home whose database cannot be read, gives the word back.
func WhoKey(home, word string) string {
	if word == "human" {
		if d, _ := store.SharedIfThere(DBFile(home)); d != nil {
			if k, err := actor.Owner(d); err == nil && k != "" {
				return k
			}
		}
		return word
	}
	if _, _, ok := actor.Parse(word); ok {
		return word
	}
	if k := AgentKey(home, word); k != "" {
		return k
	}
	return word
}

// AgentHandle is the agent's name anyone can type for it: its name when that resolves back to it (AgentKeyIn), else "workspace/name".
// key may be any word AgentKeyIn takes; it comes back as it is when neither resolves back or the agents table cannot say.
func AgentHandle(home, key string) string {
	d, err := store.SharedIfThere(DBFile(home))
	if d == nil || err != nil {
		return key
	}
	k, err := AgentKeyIn(d, key)
	_, id, ok := actor.Parse(k)
	if err != nil || !ok {
		return key
	}
	var name, ws string
	if err := d.QueryRow(`SELECT a.name, w.name FROM agents a JOIN workspaces w ON w.id = a.workspace WHERE a.id = ?`, id).Scan(&name, &ws); err != nil {
		return key
	}
	for _, h := range []string{name, ws + "/" + name} {
		if back, err := AgentKeyIn(d, h); err == nil && back == k {
			return h
		}
	}
	return key
}

// AgentKeyReadOnly is AgentKeyIn on home's database opened read-only and closed after: for hl, which must not migrate the daemon's
// database by asking who it is.
func AgentKeyReadOnly(home, who string) (string, error) {
	d, err := sql.Open("sqlite", "file:"+DBFile(home)+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer d.Close()
	return AgentKeyIn(d, who)
}
