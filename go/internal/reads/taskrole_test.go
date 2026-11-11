package reads

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

// Task.role and AgentView.task_role are check for an agent whose row says role check, and work for any other, or for a name that is no
// agent. The name does not decide it.
func TestATaskOfACheckAgentIsACheck(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	teamtest.Seed(t, e.Home, "auditor", "dev", "dev.auditor", "odd")
	teamtest.Set(t, e.Home, "auditor", "role", "check")
	nameOf := map[string]string{} // by key, so the wants read by name
	key := func(name string) string { k := teamtest.Key(t, e.Home, name); nameOf[k] = name; return k }
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', ?, ?, 1), (2, 'b', ?, ?, 1), "+
		"(3, 'c', ?, ?, 1), (4, 'd', 'nobody', ?, 1), (5, 'e', ?, ?, 1)", key("auditor"), key("dev"), key("dev"), key("dev"),
		key("dev.auditor"), key("dev"), key("dev"), key("odd"), key("dev"))
	nameOf["nobody"] = "nobody"
	got, err := e.tasksList(asFounder, map[string]any{"all": true})
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]any{}
	for _, it := range got.(map[string]any)["items"].([]any) {
		row := it.(map[string]any)
		roles[nameOf[fmt.Sprint(row["to"])]] = row["role"]
	}
	if want := "map[auditor:check dev:work dev.auditor:work nobody:work odd:work]"; fmt.Sprint(roles) != want {
		t.Errorf("Task.role by owner %v, want %s", roles, want)
	}
	list, err := e.agentsList(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(list)
	var out struct{ Agents []map[string]any }
	json.Unmarshal(b, &out)
	views := map[string]any{}
	for _, a := range out.Agents {
		views[nameOf[fmt.Sprint(a["key"])]] = a["task_role"]
	}
	delete(roles, "nobody")
	if fmt.Sprint(views) != fmt.Sprint(roles) {
		t.Errorf("AgentView.task_role %v, want %v", views, roles)
	}
}
