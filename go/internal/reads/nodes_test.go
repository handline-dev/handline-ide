package reads

import (
	"encoding/json"
	"testing"

	"github.com/handline/handline/go/data/db"
)

func TestNodeStatusListsThisMachineFirstAsMasterSelfAndOnlineThenTheMachinesOnRunOrPause(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	id, err := db.NodeID(e.Home, e.DB)
	if err != nil {
		t.Fatal(err)
	}
	run(t, e, "INSERT INTO nodes(id, person, name, joined_at, lending) VALUES('aaaa', 1, 'box', 50, 'pause')") // sorts first by id, joined first
	run(t, e, "INSERT INTO nodes(id, person, name, joined_at, lending) VALUES('bbbb', 1, 'run', 60, 'run')")
	run(t, e, "INSERT INTO nodes(id, person, name, joined_at) VALUES('cccc', 1, 'off', 40)")
	if err := db.KeepOwnNode(e.DB, id, "mac", 100); err != nil {
		t.Fatal(err)
	}
	v, err := e.nodeStatus(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	want := `{"master":{"id":"` + id + `","number":1},"nodes":[` +
		`{"id":"` + id + `","person":"p1","name":"mac","joined_at":"1970-01-01T00:01:40Z","last_seen":"1970-01-01T00:01:40Z","master_number":1,"master":true,"self":true,"online":true,"lending":"run"},` +
		`{"id":"aaaa","person":"p1","name":"box","joined_at":"1970-01-01T00:00:50Z","last_seen":null,"master_number":1,"master":false,"self":false,"online":false,"lending":"pause"},` +
		`{"id":"bbbb","person":"p1","name":"run","joined_at":"1970-01-01T00:01:00Z","last_seen":null,"master_number":1,"master":false,"self":false,"online":false,"lending":"run"}]}`
	if !sameJSON(string(b), want) {
		t.Fatalf("%s", b)
	}
}
