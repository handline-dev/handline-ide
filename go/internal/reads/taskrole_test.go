package reads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Task.role and AgentView.task_role are check for an agent whose team.yaml entry says role: check, and work for any other: one with
// another role, none, or no entry. The name does not decide it.
func TestATaskOfACheckAgentIsACheck(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	team := "agents:\n  auditor: {role: check}\n  dev: {}\n  dev.auditor: {}\n  odd: {role: review}\n"
	if err := os.WriteFile(filepath.Join(e.Home, "team.yaml"), []byte(team), 0o644); err != nil {
		t.Fatal(err)
	}
	names := []string{"auditor", "dev", "dev.auditor", "nobody", "odd"}
	for _, n := range names {
		addAgent(t, e, n, "")
	}
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'a', 'auditor', 'dev', 1), (2, 'b', 'dev', 'dev', 1), "+
		"(3, 'c', 'dev.auditor', 'dev', 1), (4, 'd', 'nobody', 'dev', 1), (5, 'e', 'odd', 'dev', 1)")
	got, err := e.tasksList(asFounder, map[string]any{"all": true})
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]any{}
	for _, it := range got.(map[string]any)["items"].([]any) {
		row := it.(map[string]any)
		roles[fmt.Sprint(row["to"])] = row["role"]
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
		views[fmt.Sprint(a["name"])] = a["task_role"]
	}
	if fmt.Sprint(views) != fmt.Sprint(roles) {
		t.Errorf("AgentView.task_role %v, want %v", views, roles)
	}
}
