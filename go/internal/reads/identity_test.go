package reads

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
)

func TestPeopleAndWorkspacesAreListedByKey(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO workspaces(id, name, folder, added_folders, created_at) VALUES(5, 'shop', '/p', '[\"/q\"]', 0)")
	run(t, e, "INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(7, 5, 'lead', 'p1', 0)")
	run(t, e, "UPDATE workspaces SET lead = 7 WHERE id = 5")
	// a seat (hired_from) never leads, and keys compare as numbers: a9 before a10
	run(t, e, "INSERT INTO agents(id, workspace, name, owner, created_at, hired_from) VALUES(8, 5, 'lead-2', 'p1', 0, 7)")
	run(t, e, "INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(10, 5, 'ui', 'p1', 0), (9, 5, 'api', 'p1', 0)")
	got := func(f func() (any, error)) string {
		v, err := f()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	ws := got(func() (any, error) { return e.workspacesList(nil, nil) })
	// next_lead: the oldest agent there but the lead, who leads it once the lead is retired
	if want := `{"key":"w5","lead":"a7","next_lead":"a9","name":"shop","folder":"/p","added_folders":["/q"],` +
		`"folders":[{"path":"/p","name":"p","git":false,"default":true},{"path":"/q","name":"q","git":false,"default":false}]}`; !sameJSON(ws, `{"workspaces":[`+want+`]}`) {
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
		t.Errorf("no standing context is no text: %s", s)
	}
	if _, err := db.SetInstruction(e.DB, "w5", db.PartRules, "Keep messages short. ", "p1", "", 1); err != nil {
		t.Fatal(err)
	}
	for _, ws := range []string{"shop", "w5"} {
		if s := got(map[string]any{"workspace": ws}); !sameJSON(s, fmt.Sprintf(shop, 20)) {
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

func TestWorkspaceRulesFollowsAnAgentToItsWorkspace(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	run(t, e, "INSERT INTO workspaces(id, name, folder, created_at) VALUES(5, 'shop', '', 0)")
	run(t, e, `INSERT INTO workspace_settings(workspace, key, value, set_by, set_at) VALUES(5, 'land_by', '"a8"', 'p1', 1)`)
	v, err := e.workspaceRules(nil, map[string]any{"agent": "nobody"})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(v); string(b) != `{"rules":[]}` {
		t.Errorf("an agent in no workspace gets the machine's: %s", b)
	}
}
