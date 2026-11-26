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

// Free takes the first word Check passes, then the first word with a number; a plain name is free in one workspace while taken in
// another, and no Plain name breaks the rules.
func TestFreePicksTheFirstWordCheckTakes(t *testing.T) {
	db := storetest.Open(t)
	for _, q := range []string{
		"INSERT INTO workspaces(name, folder, created_at) VALUES('shop', '/x', 0)",
		"INSERT INTO workspaces(name, folder, created_at) VALUES('workspace', '/y', 0)",
		"INSERT INTO workspaces(name, folder, created_at) VALUES('workspace-2', '/z', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(1, 'ada', 'p1', 0)",
		"INSERT INTO workspaces(name, folder, created_at) VALUES('" + strings.Repeat("x", 32) + "', '/w', 0)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		kind  Kind
		ws    int64
		words []string
		want  string
	}{
		{Agent, 1, Plain, "bea"},
		{Agent, 2, Plain, "ada"},
		{Workspace, 0, []string{"workspace"}, "workspace-3"},
		{Workspace, 0, []string{"shop", "workspace"}, "shop-2"},
		{Workspace, 0, []string{strings.Repeat("x", 32)}, strings.Repeat("x", 28) + "-2"},
	} {
		if got, err := Free(db, c.kind, c.ws, c.words...); err != nil || got != c.want {
			t.Errorf("%v %v: %q %v, want %q", c.kind, c.words, got, err, c.want)
		}
	}
	empty := storetest.Open(t)
	for _, w := range Plain {
		if err := Check(empty, Agent, w, 1); err != nil {
			t.Errorf("plain name %q: %v", w, err)
		}
	}
}

func TestFromMakesAWordAName(t *testing.T) {
	for in, want := range map[string]string{"My Project": "my-project", "handline-ide": "handline-ide", "2024_Site.v2": "site-v2",
		"한글": "", strings.Repeat("ab", 20): strings.Repeat("ab", 16), "--x--": "x"} {
		if got := From(in); got != want {
			t.Errorf("From(%q) = %q, want %q", in, got, want)
		}
	}
}
