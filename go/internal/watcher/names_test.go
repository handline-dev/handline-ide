package watcher

import (
	"encoding/json"
	"testing"

	"github.com/handline/handline/go/data/stream"
)

func TestARenameIsAgentChangedAndAWriteToPeopleOrWorkspacesIsTheirChanged(t *testing.T) {
	d, l := rig(t)
	exec(t, d, "INSERT INTO workspaces(id, name, folder, created_at) VALUES(5, 'shop', '/p', 0)")
	exec(t, d, "INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(7, 5, 'designer', 'p1', 0)")
	exec(t, d, `INSERT INTO agent_view(agent, view, updated) VALUES('a7', '{"name":"a7","state":"idle"}', 0)`)
	emit := func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }
	dets := []Detector{&Agents{}, People(), Workspaces(), Nodes()}
	scan := func() {
		for _, x := range dets {
			if err := x.Scan(d, emit); err != nil {
				t.Fatal(err)
			}
		}
	}
	all := func() []*stream.Event {
		got, err := l.Since(0, []string{"agents", "people", "workspaces", "nodes"}, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	scan()
	if got := all(); len(got) != 0 {
		t.Fatalf("the first look said something: %v", got)
	}
	exec(t, d, "UPDATE agents SET name = 'painter' WHERE id = 7")
	scan()
	got := all()
	var v map[string]any
	if len(got) != 2 || got[0].Type != "agent.changed" || got[1].Type != "nodes.changed" || json.Unmarshal(got[0].Data, &v) != nil || // nodes: its placement
		v["key"] != "a7" || v["name"] != "painter" || v["workspace"] != "w5" || v["owner"] != "p1" {
		t.Fatalf("a rename: %+v %v", got, v)
	}
	exec(t, d, "UPDATE workspaces SET name = 'store' WHERE id = 5")
	exec(t, d, "INSERT INTO people(display_name, role, created_at) VALUES('Ron', 'owner', 0)")
	scan()
	got = all()[2:]
	if len(got) != 2 || got[0].Type != "people.changed" || got[1].Type != "workspaces.changed" || string(got[0].Data) != "{}" {
		t.Fatalf("%+v", got)
	}
	exec(t, d, "INSERT INTO nodes(id, person, name, joined_at) VALUES('n1', 1, 'mac', 0)")
	scan()
	if got = all()[4:]; len(got) != 1 || got[0].Type != "nodes.changed" {
		t.Fatalf("a node row: %+v", got)
	}
	scan()
	if got = all()[5:]; len(got) != 0 {
		t.Fatalf("no write said something: %+v", got)
	}
}
