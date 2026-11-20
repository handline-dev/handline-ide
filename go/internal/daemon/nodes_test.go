package daemon

import (
	"testing"
)

// The master's own row is written at start under the node id daemon.status shows, once: a restart keeps it and its join time.
func TestNodeStatusListsThisMachineAsTheMastersOwnRowAcrossARestart(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth = pidAuth{}
	look := func() (status, nodes map[string]any) {
		d, err := Start(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		f := readAll(t, pipeTo(t, d, "k"), 2, 3)
		status = f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"daemon.status","params":{}}`)["result"].(map[string]any)
		nodes = f.ask(3, `{"jsonrpc":"2.0","id":3,"method":"node.status","params":{}}`)["result"].(map[string]any)
		t.Logf("node.status: %v", nodes)
		return status, nodes
	}
	status, first := look()
	_, again := look()
	id := status["machine"].(map[string]any)["id"]
	list := again["nodes"].([]any)
	own := list[0].(map[string]any)
	if len(list) != 1 || own["id"] != id || again["master"].(map[string]any)["id"] != id || own["person"] != "p1" ||
		own["master"] != true || own["self"] != true || own["online"] != true || own["master_number"] != float64(1) {
		t.Fatalf("%v (daemon.status machine %v)", again, id)
	}
	if was := first["nodes"].([]any)[0].(map[string]any)["joined_at"]; own["joined_at"] != was {
		t.Fatalf("a restart moved joined_at: %v then %v", was, own["joined_at"])
	}
}
