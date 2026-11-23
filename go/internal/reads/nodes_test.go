package reads

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/version"
)

func TestNodeStatusListsThisMachineFirstAsMasterSelfAndOnlineThenTheMachinesOnRunOrPause(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	id, err := db.NodeID(e.Home, e.DB)
	if err != nil {
		t.Fatal(err)
	}
	run(t, e, "INSERT INTO nodes(id, person, name, joined_at, lending) VALUES('aaaa', 1, 'box', 50, 'pause')") // sorts first by id, joined first
	run(t, e, "INSERT INTO nodes(id, person, name, joined_at, lending, door, door_fp) VALUES('bbbb', 1, 'run', 60, 'run', '10.0.0.2:7777', 'ab')")
	run(t, e, "INSERT INTO nodes(id, person, name, joined_at) VALUES('cccc', 1, 'off', 40)")
	if err := db.KeepOwnNode(e.DB, id, "mac", 100); err != nil {
		t.Fatal(err)
	}
	v, err := e.nodeStatus(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	none := `"device":null,"memory_max_gb":null,"load":null,"battery":null,"set_aside":null,"version":null,"app":null`
	self := `"device":null,"memory_max_gb":null,"load":null,"battery":null,"set_aside":null,"version":"` + version.Sha + `","app":null`
	want := `{"cluster":{"machines":3,"name":""},"master":{"id":"` + id + `","number":1},"nodes":[` +
		`{"id":"` + id + `","person":"p1","name":"mac","joined_at":"1970-01-01T00:01:40Z","last_seen":"1970-01-01T00:01:40Z","master_number":1,"master":true,"self":true,"online":true,"lending":"run","door":null,` + self + `},` +
		`{"id":"aaaa","person":"p1","name":"box","joined_at":"1970-01-01T00:00:50Z","last_seen":null,"master_number":1,"master":false,"self":false,"online":false,"lending":"pause","door":null,` + none + `},` +
		`{"id":"bbbb","person":"p1","name":"run","joined_at":"1970-01-01T00:01:00Z","last_seen":null,"master_number":1,"master":false,"self":false,"online":false,"lending":"run","door":{"addr":"10.0.0.2:7777","fp":"ab"},` + none + `}]}`
	if !sameJSON(string(b), want) {
		t.Fatalf("%s", b)
	}
	e.NodeHeard = func(node string) (map[string]any, bool, string, bool) { return nil, false, "older1", node == "bbbb" }
	v, err = e.nodeStatus(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(v)
	var got struct {
		Nodes []struct {
			Version *string `json:"version"`
		} `json:"nodes"`
	}
	if json.Unmarshal(b, &got) != nil || len(got.Nodes) != 3 || got.Nodes[1].Version != nil || got.Nodes[2].Version == nil || *got.Nodes[2].Version != "older1" {
		t.Fatalf("a runner's version is its heartbeat's: %s", b)
	}
	e.NodeApp = func(node string) any {
		if node != "bbbb" {
			return nil
		}
		return map[string]any{"version": "0.1.16", "ready": "0.1.17"}
	}
	v, _ = e.nodeStatus(nil, nil)
	b, _ = json.Marshal(v)
	if !strings.Contains(string(b), `"app":{"ready":"0.1.17","version":"0.1.16"}`) {
		t.Fatalf("a runner's Mac app is its heartbeat's: %s", b)
	}
}
