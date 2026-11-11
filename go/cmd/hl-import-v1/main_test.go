package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/store"
)

// oldHome writes an old home: two workspaces' agents by dotted name, an account with its settings in config.yaml, team rules, and a
// database of the old shape (the base schema with devices before they had a person) holding rows that name them.
func oldHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(h, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("agents/shop/agent.yaml", "name: shop\nmodel: opus\nrepo: /src/shop\nsandbox: false\n")
	write("agents/shop.ui/agent.yaml", "name: shop.ui\nmodel: sonnet\nenabled: true\ncontext_tokens: 600000\nrepo: /src/other\nskip_permissions: true\n")
	write("agents/shop.ui/memory/note.md", "kept")
	write("agents/home/agent.yaml", "name: home\nrepo: /src/home\n")
	write("agents/shop.gone/inbox/x", "") // no agent.yaml: no agent now
	write("accounts/alpha/identity.json", "{}")
	write("config.yaml", "version: 1\nactive_cap: 2\nactive_cap_alpha: 3\nmonth_cycle_alpha: false\n")
	write("team.yaml", "agents:\n  shop.ui: { role: check }\n")
	write("shared/team/shop.md", "shop rules\n")
	write("shared/team/shop.sub.md", "sub rules\n")
	write("attachments/a.png", "png")

	d, err := store.Open(filepath.Join(h, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{
		"ALTER TABLE devices DROP COLUMN person",
		`INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(9, 'its parent', 'shop.gone', 'task-board', 1)`,
		// read back by id, so the import meets the task before its parent
		`INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(7, 'a task', 'shop.ui', 'human', 1, 9)`,
		`INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(3, 7, 'shop', 'ask @shop.ui. and @human, not a@shop.ui or @nobody', 1)`,
		`INSERT INTO requests(id, ts, requester, target, body) VALUES(5, 1, 'founder', 'shop', 'hi @home')`,
		`INSERT INTO requests(id, ts, requester, target, body) VALUES(6, 1, 'land', 'shop', 'landed')`,
		`INSERT INTO prefs(person, nav, updated_at) VALUES('human', '{"pins":["shop.ui","shop.gone","human"],"hidden":["idle"],"layout":"tree"}', 1)`,
		`INSERT INTO devices(id, name, token_hash, created) VALUES('dev_1', 'phone', 'h', 1)`,
		`INSERT INTO agent_state(agent, state, updated, worktree) VALUES('shop.ui', 'RUNNING', 1, '/src/shop.worktrees/shop.ui')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return h
}

func TestImportWritesTheNewHome(t *testing.T) {
	from := oldHome(t)
	to := filepath.Join(t.TempDir(), "new")
	log, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	if err := run(from, to, "Ron", log); err != nil {
		t.Fatal(err)
	}
	d, err := sql.Open("sqlite", "file:"+filepath.Join(to, "db.sqlite")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	one := func(q string) string {
		t.Helper()
		var s sql.NullString
		if err := d.QueryRow(q).Scan(&s); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return s.String
	}
	ui := "a" + one("SELECT a.id FROM agents a JOIN workspaces w ON w.id = a.workspace WHERE w.name = 'shop' AND a.name = 'ui'")
	lead := "a" + one("SELECT lead FROM workspaces WHERE name = 'shop'")
	home := "a" + one("SELECT lead FROM workspaces WHERE name = 'home'")
	for q, want := range map[string]string{
		"SELECT name FROM agents WHERE id = " + lead[1:]:                                                       "lead",
		"SELECT folder FROM workspaces WHERE name = 'shop'":                                                    "/src/shop",
		"SELECT reports_to FROM agents WHERE id = " + lead[1:]:                                                 "p1",
		"SELECT reports_to FROM agents WHERE id = " + ui[1:]:                                                   lead,
		"SELECT role || model || context_tokens || enabled FROM agents WHERE id = " + ui[1:]:                   "checksonnet6000001",
		"SELECT sandbox FROM agents WHERE id = " + lead[1:]:                                                    "0",
		"SELECT COUNT(*) FROM agents":                                                                          "3",
		"SELECT owner || ' ' || opened_by || ' ' || parent_task FROM tasks WHERE id = 7":                       ui + " p1 9",
		"SELECT owner || ' ' || opened_by FROM tasks WHERE id = 9":                                             "shop.gone sys",
		"SELECT author FROM task_memos WHERE id = 3":                                                           lead,
		"SELECT body FROM task_memos WHERE id = 3":                                                             "ask <@" + ui + ">. and <@p1>, not a@shop.ui or @nobody",
		"SELECT requester || ' ' || target || ' ' || body FROM requests WHERE id = 5":                          "p1 " + lead + " hi <@" + home + ">",
		"SELECT requester FROM requests WHERE id = 6":                                                          "sys",
		"SELECT person || ' ' || nav FROM prefs":                                                               `p1 {"hidden":["idle"],"layout":"tree","pins":["` + ui + `"]}`,
		"SELECT person FROM devices WHERE id = 'dev_1'":                                                        "p1",
		"SELECT agent || ' ' || state || ' ' || worktree FROM agent_state":                                     ui + " IDLE /src/shop.worktrees/shop.ui",
		"SELECT name || ' ' || label || ' ' || active_cap || ' ' || month_cycle || ' ' || owner FROM accounts": "alpha alpha 3 0 p1",
	} {
		if got := one(q); got != want {
			t.Errorf("%s = %q, want %q", q, got, want)
		}
	}
	files := map[string]string{
		"agents/" + ui + "/memory/note.md": "kept",
		"accounts/c1/identity.json":        "{}",
		"workspaces/w2/base.md":            "shop rules\n",
		"attachments/a.png":                "png",
		"config.yaml":                      "version: 1\nactive_cap: 2\n",
	}
	if one("SELECT id FROM workspaces WHERE name = 'shop'") != "2" {
		t.Fatal("workspaces are made by name, home before shop")
	}
	for rel, want := range files {
		b, err := os.ReadFile(filepath.Join(to, rel))
		if err != nil || string(b) != want {
			t.Errorf("%s = %q, %v; want %q", rel, b, err, want)
		}
	}
	for _, gone := range []string{"shared/team", "agents/shop.gone", "agents/shop.ui"} {
		if _, err := os.Stat(filepath.Join(to, gone)); err == nil {
			t.Errorf("%s was brought", gone)
		}
	}
	b, _ := os.ReadFile(log.Name())
	for _, said := range []string{"skip_permissions=true is no column", "tasks.owner: kept as text, no one now: shop.gone (1)",
		"prefs.nav.pins: shop.gone names no agent now"} {
		if !strings.Contains(string(b), said) {
			t.Errorf("the report does not say %q:\n%s", said, b)
		}
	}
}

func TestImportRefusesANewHomeThatHoldsAnything(t *testing.T) {
	from := oldHome(t)
	to := t.TempDir()
	if err := os.WriteFile(filepath.Join(to, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(from, to, "Ron", os.Stderr); err == nil || !strings.Contains(err.Error(), "is not empty") {
		t.Fatalf("run = %v, want a refusal of a home that holds something", err)
	}
	if _, err := os.Stat(filepath.Join(to, "db.sqlite")); err == nil {
		t.Fatal("a refused import wrote a database")
	}
}
