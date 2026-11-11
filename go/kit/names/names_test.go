package names

import (
	"errors"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/storetest"
)

func TestCheck(t *testing.T) {
	db := storetest.Open(t)
	for _, q := range []string{
		"INSERT INTO people(display_name, handle, role, created_at) VALUES('Jisoo', 'jisoo', 'member', 0)",
		"INSERT INTO workspaces(name, folder, created_at) VALUES('shop', '/x', 0)",
		"INSERT INTO workspaces(name, folder, created_at) VALUES('blog', '/y', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(1, 'designer', 'p1', 0)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		kind Kind
		word string
		ws   int64
		why  string // "" when allowed
	}{
		{Agent, "reviewer", 1, ""},
		{Agent, "designer", 2, ""}, // the same name in another workspace
		{Agent, "designer", 1, "an agent of this workspace"},
		{Agent, "blog", 1, "a workspace"},
		{Agent, "jisoo", 1, "a person's handle"},
		{Workspace, "docs", 0, ""},
		{Workspace, "shop", 0, "a workspace"},
		{Workspace, "designer", 0, "an agent"},
		{Workspace, "jisoo", 0, "a person's handle"},
		{Handle, "mina", 0, ""},
		{Handle, "jisoo", 0, "a person's handle"},
		{Handle, "designer", 0, "an agent"},
		{Handle, "shop", 0, "a workspace"},
		{Agent, "human", 1, "reserved"},
		{Workspace, "everyone", 0, "reserved"},
		{Agent, "Designer", 1, "lowercase"},
		{Agent, "shop.ui", 1, "lowercase"},
		{Agent, "9lives", 1, "starting with a letter"},
		{Agent, "", 1, "lowercase"},
		{Agent, strings.Repeat("a", 33), 1, "1 to 32"},
		{Agent, strings.Repeat("a", 32), 1, ""},
		{Agent, "ui-review2", 1, ""},
		{Agent, "a17", 1, "reads as a key"},
		{Handle, "p2", 0, "reads as a key"},
		{Agent, "ab17", 1, ""},
	} {
		err := Check(db, c.kind, c.word, c.ws)
		var r *Refusal
		switch {
		case c.why == "" && err != nil:
			t.Errorf("%v %q: refused: %v", c.kind, c.word, err)
		case c.why != "" && (!errors.As(err, &r) || !strings.Contains(r.Msg, c.why)):
			t.Errorf("%v %q: got %v, want a refusal naming %q", c.kind, c.word, err, c.why)
		}
	}
}
