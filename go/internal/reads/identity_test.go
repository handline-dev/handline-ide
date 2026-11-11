package reads

import (
	"encoding/json"
	"testing"
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
