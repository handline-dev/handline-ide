package addr

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/storetest"
)

// seed is two workspaces: shop (lead a1 "lead", a2 "designer") in /p/shop, and blog (lead a3 "lead", a4 "designer") in /p/blog; the
// owner p1 and a member p2 with handle jisoo.
func seed(t *testing.T) *sql.DB {
	t.Helper()
	db := storetest.Open(t)
	for _, q := range []string{
		"INSERT INTO people(display_name, handle, role, created_at) VALUES('Jisoo', 'jisoo', 'member', 0)",
		"INSERT INTO workspaces(name, folder, created_at) VALUES('shop', '/p/shop', 0)",
		"INSERT INTO workspaces(name, folder, created_at) VALUES('blog', '/p/blog', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(1, 'lead', 'p1', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(1, 'designer', 'p1', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(2, 'lead', 'p2', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(2, 'designer', 'p1', 0)",
		"UPDATE workspaces SET lead = 1 WHERE id = 1",
		"UPDATE workspaces SET lead = 3 WHERE id = 2",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	return db
}

func TestResolve(t *testing.T) {
	db := seed(t)
	shopAgent := Who{Key: "a2", Workspace: 1}
	blogLead := Who{Key: "a3", Workspace: 2}
	outside := Who{Key: "p1"}
	for _, c := range []struct {
		who  Who
		word string
		want string // "" when refused
	}{
		{shopAgent, "human", "p1"},
		{blogLead, "human", "p2"}, // an agent's owner, not the install's
		{outside, "human", "p1"},
		{shopAgent, "lead", "a1"},
		{blogLead, "designer", "a4"}, // its own workspace's, not shop's
		{shopAgent, "blog", "a3"},    // a workspace is its lead
		{shopAgent, "shop", "a1"},
		{shopAgent, "jisoo", "p2"},
		{shopAgent, "a4", "a4"}, // a key that names a row
		{shopAgent, "sys", "sys"},
		{shopAgent, "a9", ""},
		{outside, "designer", ""}, // a person outside every folder has no agent names
		{outside, "blog", "a3"},
		{shopAgent, "blog.designer", ""}, // an old dotted name
		{shopAgent, "founder", ""},
		{shopAgent, "nobody", ""},
	} {
		got, err := Resolve(db, c.who, c.word)
		var r *Refusal
		if c.want == "" {
			if !errors.As(err, &r) {
				t.Errorf("%v %q: %q %v, want a refusal", c.who, c.word, got, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%v %q: %q %v, want %q", c.who, c.word, got, err, c.want)
		}
	}
}

func TestRefusalListsTheNamesTheCallerMayUse(t *testing.T) {
	db := seed(t)
	_, err := Resolve(db, Who{Key: "a2", Workspace: 1}, "nobody")
	want := `"nobody" names no one you can reach: use human, or an agent of your workspace (designer, lead), or a workspace, whose lead receives it (blog), or a person (jisoo)`
	if err == nil || err.Error() != want {
		t.Fatalf("got %v\nwant %s", err, want)
	}
}

func TestOfFindsAPersonsWorkspaceByFolder(t *testing.T) {
	db := seed(t)
	if _, err := db.Exec("INSERT INTO workspaces(name, folder, created_at) VALUES('inner', '/p/shop/inner', 0)"); err != nil {
		t.Fatal(err)
	}
	for cwd, want := range map[string]int64{"/p/shop": 1, "/p/shop/go/x": 1, "/p/shop/inner/a": 3, "/p/shopper": 0, "": 0, "/elsewhere": 0} {
		w, err := Of(db, "p1", cwd)
		if err != nil || w.Workspace != want {
			t.Errorf("%q: %+v %v, want workspace %d", cwd, w, err, want)
		}
	}
	if w, _ := Of(db, "a4", "/p/shop"); w.Workspace != 2 {
		t.Errorf("an agent's workspace is its own, not its cwd's: %+v", w)
	}
}

func TestName(t *testing.T) {
	db := seed(t)
	shopAgent := Who{Key: "a2", Workspace: 1}
	for key, want := range map[string]string{
		"a1":  "lead",
		"a2":  "designer",
		"a3":  "blog", // another workspace's lead is that workspace, which resolves
		"a4":  "designer (blog)",
		"p1":  "human",
		"p2":  "jisoo",
		"w2":  "blog",
		"sys": "task-board",
		"a99": "a99",
		"hi":  "hi",
	} {
		if got := Name(db, shopAgent, key); got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
	if got := Name(db, Who{Key: "a3", Workspace: 2}, "p2"); got != "human" {
		t.Errorf("p2 to its own agent: %q, want human", got)
	}
	if got := Name(db, Who{Key: "a3", Workspace: 2}, "p1"); got != "Owner" {
		t.Errorf("a person with no handle: %q, want the display name", got)
	}
}

func TestMentionsAreStoredByKeyAndWrittenOutPerReader(t *testing.T) {
	db := seed(t)
	text := "@human see @designer's note, ask @blog. Not @nobody, a@b.c or `x@lead`."
	stored, err := Mentions(db, Who{Key: "a1", Workspace: 1}, text)
	want := "<@p1> see <@a2>'s note, ask <@a3>. Not @nobody, a@b.c or `x@lead`."
	if err != nil || stored != want {
		t.Fatalf("stored %q %v\nwant   %q", stored, err, want)
	}
	if got := Refs(stored); strings.Join(got, ",") != "p1,a2,a3" {
		t.Errorf("refs %v", got)
	}
	if got := Lead(stored); got != "p1" {
		t.Errorf("lead %q", got)
	}
	if got := Lead("hi " + stored); got != "" {
		t.Errorf("a mention past the first token leads nothing: %q", got)
	}
	got := Render(db, Who{Key: "a4", Workspace: 2}, stored)
	if want := "@human see @designer (shop)'s note, ask @lead. Not @nobody, a@b.c or `x@lead`."; got != want {
		t.Errorf("blog's designer reads %q\nwant %q", got, want)
	}
}

// A hint names a who by a word the reader can type back: the name when it resolves to the key, else the key.
func TestAddressIsAWordThatResolvesBack(t *testing.T) {
	db := seed(t)
	for key, want := range map[string]string{"a1": "lead", "a3": "blog", "a4": "a4", "p1": "human", "sys": "sys"} {
		if got := Address(db, "a2", key); got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
}

// RenderFor writes the refs out for the reader by its key: its own workspace's agents by name, another's lead by its workspace.
func TestRenderForWritesRefsOutForOneReader(t *testing.T) {
	db := seed(t)
	if got := RenderFor(db, "a4", "blocked by T7 (<@a3>)"); got != "blocked by T7 (@lead)" {
		t.Errorf("%q", got)
	}
	if got := RenderFor(db, "a1", "T7 (<@a3>)"); got != "T7 (@blog)" {
		t.Errorf("shop's lead: %q", got)
	}
}
