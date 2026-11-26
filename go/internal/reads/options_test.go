package reads

import (
	"testing"

	"github.com/handline/handline/go/kit/names"
)

// agent.options names, for each workspace, the plain name agent.create gives an agent made there with none typed.
func TestAgentOptionsSaysEachWorkspacesDefaultName(t *testing.T) {
	e := testEnv(t)
	for _, q := range []string{
		"INSERT INTO workspaces(name, folder, created_at) VALUES('shop', '/x', 0)",
		"INSERT INTO workspaces(name, folder, created_at) VALUES('blog', '/y', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(1, '" + names.Plain[0] + "', 'p1', 0)",
	} {
		if _, err := e.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.agentOptions(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := got.(map[string]any)["default_names"].(map[string]any)
	if d["w1"] != names.Plain[1] || d["w2"] != names.Plain[0] || len(d) != 2 {
		t.Fatalf("%v", d)
	}
}
