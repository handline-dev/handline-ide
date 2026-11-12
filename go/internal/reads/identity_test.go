package reads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team"
)

func TestPeopleAndWorkspacesAreListedByKey(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO workspaces(id, name, folder, added_folders, created_at) VALUES(5, 'shop', '/p', '[\"/q\"]', 0)")
	run(t, e, "INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(7, 5, 'lead', 'p1', 0)")
	run(t, e, "UPDATE workspaces SET lead = 7 WHERE id = 5")
	got := func(f func() (any, error)) string {
		v, err := f()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	ws := got(func() (any, error) { return e.workspacesList(nil, nil) })
	if want := `{"key":"w5","lead":"a7","name":"shop","folder":"/p","added_folders":["/q"]}`; !sameJSON(ws, `{"workspaces":[`+want+`]}`) {
		t.Errorf("workspaces.list %s", ws)
	}
	people := got(func() (any, error) { return e.peopleList(nil, nil) })
	var p struct{ People []map[string]any }
	json.Unmarshal([]byte(people), &p)
	if len(p.People) == 0 || p.People[0]["key"] != "p1" || p.People[0]["role"] != "owner" || p.People[0]["handle"] != nil {
		t.Errorf("people.list %s", people)
	}
}

func sameJSON(a, b string) bool {
	var x, y any
	json.Unmarshal([]byte(a), &x)
	json.Unmarshal([]byte(b), &y)
	x1, _ := json.Marshal(x)
	y1, _ := json.Marshal(y)
	return string(x1) == string(y1)
}

func TestWorkspaceRulesListsTheRulesInForceAndWhereEachComesFrom(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO workspaces(id, name, folder, created_at) VALUES(5, 'shop', '', 0)")
	run(t, e, `INSERT INTO workspace_settings(workspace, key, value, set_by, set_at) VALUES(0, 'agent_cap', '3', 'p1', 1), `+
		`(0, 'tools_deny', '["WebFetch"]', 'p1', 1), (5, 'agent_cap', '0', 'p1', 2)`)
	got := func(params map[string]any) string {
		v, err := e.workspaceRules(nil, params)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	shop := `{"rules":[{"key":"agent_cap","value":0,"on":false,"source":"workspace","set_by":"p1","set_at":2},` +
		`{"key":"tools_deny","value":["WebFetch"],"on":true,"source":"machine","set_by":"p1","set_at":1}],"limit_bytes":2048,"text_bytes":%d}`
	if s := got(map[string]any{"workspace": "shop"}); !sameJSON(s, fmt.Sprintf(shop, 0)) {
		t.Errorf("no base.md is no text: %s", s)
	}
	base := team.RulesFile(e.Home, "w5")
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base, []byte("Keep messages short.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ws := range []string{"shop", "w5"} {
		if s := got(map[string]any{"workspace": ws}); !sameJSON(s, fmt.Sprintf(shop, 21)) {
			t.Errorf("%s: %s", ws, s)
		}
	}
	if s := got(nil); !sameJSON(s, `{"rules":[{"key":"agent_cap","value":3,"on":true,"source":"machine","set_by":"p1","set_at":1},`+
		`{"key":"tools_deny","value":["WebFetch"],"on":true,"source":"machine","set_by":"p1","set_at":1}]}`) {
		t.Errorf("the machine's: %s", s)
	}
	if _, err := e.workspaceRules(nil, map[string]any{"workspace": "nope"}); err == nil {
		t.Error("an unknown workspace is not_found")
	}
}
