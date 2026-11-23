package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
)

// joinAs trades a code for a token as a lending machine: kind and node as hl device join gives them.
func joinAs(t *testing.T, d *Daemon, code, name, kind, node string) map[string]any {
	t.Helper()
	return dialNet(t, d).ask(`{"jsonrpc":"2.0","id":1,"method":"device.redeem","params":{"code":"` + code + `","name":"` + name +
		`","kind":"` + kind + `","node":"` + node + `","request_id":"r-` + name + `"}}`)
}

func lent(t *testing.T, c *wsClient) map[string]string {
	t.Helper()
	r := c.ask(`{"jsonrpc":"2.0","id":5,"method":"node.status","params":{}}`)["result"].(map[string]any)
	out := map[string]string{}
	for _, n := range r["nodes"].([]any)[1:] { // the first is the master's own
		row := n.(map[string]any)
		out[row["id"].(string)] = row["person"].(string) + " " + row["lending"].(string)
	}
	return out
}

func set(c *wsClient, device, lending string) map[string]any {
	return c.ask(`{"jsonrpc":"2.0","id":6,"method":"device.set","params":{"device":"` + device + `","lending":"` + lending +
		`","request_id":"set-` + device + `-` + lending + `"}}`)
}

func TestAJoinedMachineIsItsPersonsDeviceOnOffAndOnlyTheyRunPauseOrTakeItBack(t *testing.T) {
	d := startNet(t)
	if _, err := d.db.Exec("INSERT INTO people(display_name, role, created_at) VALUES('Mia', 'owner', 0)"); err != nil {
		t.Fatal(err)
	}
	code, _, _, _ := d.dev.Pair(time.Minute)
	r := joinAs(t, d, code, "box", "server", "n-box")
	box, _ := r["result"].(map[string]any)
	if box == nil {
		t.Fatalf("redeem: %v", r)
	}
	door, _, _ := d.dev.Doors("")
	miaCode, _, _, _ := d.dev.PairAt(time.Minute, door, "p2")
	mia := joinAs(t, d, miaCode, "mia-mac", "mac", "n-mia")["result"].(map[string]any)

	owner := helloAs(t, d, box["token"].(string))
	list := owner.ask(`{"jsonrpc":"2.0","id":2,"method":"devices.list","params":{}}`)["result"].(map[string]any)["devices"].([]any)
	for _, v := range list {
		if dev := v.(map[string]any); dev["lending"] != "off" {
			t.Fatalf("a joined machine is not on Off: %v", dev)
		}
	}
	if got := lent(t, owner); len(got) != 0 {
		t.Fatalf("a machine on Off is listed: %v", got)
	}

	for _, step := range []struct{ lending, want string }{{"run", "p1 run"}, {"pause", "p1 pause"}} {
		if r := set(owner, box["device"].(string), step.lending); r["result"] == nil {
			t.Fatalf("%s: %v", step.lending, r)
		}
		if got := lent(t, owner); got["n-box"] != step.want || len(got) != 1 {
			t.Fatalf("%s: %v", step.lending, got)
		}
	}
	if r := set(owner, mia["device"].(string), "run"); gate(r) != "person" {
		t.Fatalf("another person set Mia's machine: %v", r)
	}
	if r := set(helloAs(t, d, mia["token"].(string)), mia["device"].(string), "run"); r["result"] == nil {
		t.Fatalf("Mia cannot run her own machine: %v", r)
	}
	if got := lent(t, owner); got["n-mia"] != "p2 run" {
		t.Fatalf("%v", got)
	}
	if r := set(owner, box["device"].(string), "off"); r["result"] == nil || lent(t, owner)["n-box"] != "" {
		t.Fatalf("Off kept it listed: %v %v", r, lent(t, owner))
	}

	again, _, _, _ := d.dev.Pair(time.Minute)
	if r := joinAs(t, d, again, "box", "server", "n-box"); gate(r) != "already_joined" {
		t.Fatalf("a machine joined twice: %v", r)
	}
	if r := joinAs(t, d, again, "box", "server", ""); r["error"] == nil {
		t.Fatalf("a server joined with no node id: %v", r)
	}
	if found, err := d.dev.Revoke(mia["device"].(string)); !found || err != nil {
		t.Fatal(found, err)
	}
	var n int
	d.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = 'n-mia'").Scan(&n)
	if n != 0 {
		t.Fatal("revoking the device kept its node row")
	}
}

// A home that joined a master sets the board it ran alone on aside at its next start, unchanged, and opens a new one; the start after
// keeps the new one.
func TestAJoinedHomeSetsItsOwnBoardAsideOnceAndOpensANewOne(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth = fakeAuth{key: "k", agent: "a"}
	start := func() *Daemon {
		d, err := Start(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	d := start()
	d.db.Exec("INSERT INTO meta(name, value, updated) VALUES('mine', 'old board', 0)")
	d.Close()

	m := joined.Master{Addr: "master:1", Token: "hld_x", Device: "dev_1", At: 7}
	if err := joined.Keep(cfg.Home, m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { joined.Forget(cfg.Home) })
	for range 2 {
		d = start()
		var v string
		if d.db.QueryRow("SELECT value FROM meta WHERE name = 'mine'").Scan(&v); v != "" {
			t.Fatal("the joined home still opens its old board")
		}
		d.Close()
	}
	aside, _ := filepath.Glob(filepath.Join(filepath.Dir(cfg.DBFile), "db.set-aside-*.sqlite"))
	if len(aside) != 1 {
		t.Fatalf("set aside: %v", aside)
	}
	if b, _ := os.ReadFile(aside[0]); !strings.Contains(string(b), "old board") {
		t.Fatal("the board set aside is not the old one, unchanged")
	}
}

// hl device join redeems the link's code at the master, pinned to its fingerprint, and keeps what it answers.
func TestJoinRedeemsTheLinkPinnedToItsFingerprintAndKeepsTheMaster(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	d := startNet(t)
	home := t.TempDir()
	t.Cleanup(func() { joined.Forget(home) })
	_, _, fp, uri := d.dev.Pair(time.Minute)
	if _, err := hlclient.Join(home, strings.Replace(uri, fp, strings.Repeat("0", len(fp)), 1), "box", "server", "n-box"); err == nil {
		t.Fatal("joined a master whose certificate is not the link's")
	}
	_, _, _, uri = d.dev.Pair(time.Minute)
	m, err := hlclient.Join(home, uri, "box", "server", "n-box")
	if err != nil {
		t.Fatal(err)
	}
	if kept, ok := joined.Of(home); !ok || kept != m || m.Addr != d.NetworkAddr() || !d.dev.Know(m.Token) {
		t.Fatalf("%+v %v", kept, ok)
	}
	var lending string
	d.db.QueryRow("SELECT lending FROM nodes WHERE device = ?", m.Device).Scan(&lending)
	if lending != "off" {
		t.Fatalf("joined on %q", lending)
	}
}

// node.status names the device each lent machine joined as, null on the master's own row, and the memory cap device.set gives it;
// load and battery are null before a heartbeat. The battery switch and the reclaim time are settings.
func TestNodeStatusNamesEachMachinesDeviceAndItsOwnMemoryCap(t *testing.T) {
	d := startNet(t)
	code, _, _, _ := d.dev.Pair(time.Minute)
	box := joinAs(t, d, code, "box", "server", "n-box")["result"].(map[string]any)
	c := helloAs(t, d, box["token"].(string))
	dev := box["device"].(string)
	row := func(id string) map[string]any {
		for _, n := range c.ask(`{"jsonrpc":"2.0","id":5,"method":"node.status","params":{}}`)["result"].(map[string]any)["nodes"].([]any) {
			if r := n.(map[string]any); r["id"] == id {
				return r
			}
		}
		t.Fatalf("no row for %s", id)
		return nil
	}
	setMem := func(v string) map[string]any {
		return c.ask(`{"jsonrpc":"2.0","id":7,"method":"device.set","params":{"device":"` + dev + `","memory_max_gb":` + v +
			`,"request_id":"mem-` + v + `"}}`)
	}
	set(c, dev, "run")
	if r := row("n-box"); r["device"] != dev || r["memory_max_gb"] != nil || r["load"] != nil || r["battery"] != nil {
		t.Fatalf("the lent machine's row: %v", r)
	}
	if r := row(joined.NodeIDOf(d.cfg.Home)); r["device"] != nil {
		t.Fatalf("the master's own row names a device: %v", r)
	}
	d.nodes.Hear("n-box", runner.Beat{Battery: true, Load: runner.Load{Cores: 8, Load1: 1.5, RAMTotal: 16 << 30, RAMFree: 4 << 30, AgentsRSS: 2 << 30}}, time.Now())
	if r := row("n-box"); r["battery"] != true || fmt.Sprint(r["load"]) != "map[agents_rss:2.147483648e+09 cores:8 load1:1.5 ram_free:4.294967296e+09 ram_total:1.7179869184e+10]" {
		t.Fatalf("after a heartbeat: load %v, battery %v", r["load"], r["battery"])
	}
	if r := setMem("6"); r["result"] == nil || row("n-box")["memory_max_gb"] != float64(6) || row("n-box")["lending"] != "run" {
		t.Fatalf("memory_max_gb 6: %v %v", r, row("n-box"))
	}
	if r := setMem("0"); r["error"] == nil {
		t.Fatalf("a cap of 0 GB was taken: %v", r)
	}
	if r := setMem("null"); r["result"] == nil || row("n-box")["memory_max_gb"] != nil {
		t.Fatalf("null did not put it back to the master's cap: %v %v", r, row("n-box"))
	}

	got := c.ask(`{"jsonrpc":"2.0","id":8,"method":"settings.get","params":{}}`)["result"].(map[string]any)["values"].(map[string]any)
	if got["node_battery_switch"] != false || got["node_reclaim_s"] != float64(1800) {
		t.Fatalf("settings.get: battery %v, reclaim %v", got["node_battery_switch"], got["node_reclaim_s"])
	}
	if r := c.ask(`{"jsonrpc":"2.0","id":9,"method":"settings.set","params":{"changes":{"node_battery_switch":true,"node_reclaim_s":600},"request_id":"s-1"}}`); r["result"] == nil {
		t.Fatalf("settings.set: %v", r)
	}
	got = c.ask(`{"jsonrpc":"2.0","id":8,"method":"settings.get","params":{}}`)["result"].(map[string]any)["values"].(map[string]any)
	if got["node_battery_switch"] != true || got["node_reclaim_s"] != float64(600) {
		t.Fatalf("after settings.set: battery %v, reclaim %v", got["node_battery_switch"], got["node_reclaim_s"])
	}
}

// device.set moves the nodes feed, and the devices topic rides with the others on one subscribe.
func TestDeviceSetMovesTheNodesFeedAndDevicesRidesWithTheOtherTopics(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth, cfg.Watch = fakeAuth{key: "k", agent: "a"}, true
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	code, _, _, _ := d.dev.Pair(time.Minute)
	box := joinAs(t, d, code, "box", "server", "n-box")["result"].(map[string]any)
	c := helloAs(t, d, box["token"].(string))
	dev := box["device"].(string)
	for i := 0; i < 2; i++ { // the client sends every topic it follows in each subscribe
		if r := c.ask(`{"jsonrpc":"2.0","id":20,"method":"subscribe","params":{"topics":["nodes","devices","status"]}}`); r["result"] == nil {
			t.Fatalf("subscribe: %v", r)
		}
	}
	c.ws.WriteMessage(websocket.TextMessage, []byte(`{"jsonrpc":"2.0","id":21,"method":"device.set","params":{"device":"`+dev+
		`","lending":"run","request_id":"set-run"}}`))
	// device.changed is live; nodes.changed comes from the watcher's next look at the board. That each comes once is
	// TestASubscribeReplacesTheConnectionsSubscriptionsItCovers's (data/stream).
	seen := map[string]int{}
	for end := time.Now().Add(15 * time.Second); seen["nodes.changed"] == 0 || seen["device.changed"] == 0; {
		m, ok := c.next(time.Until(end))
		if !ok {
			t.Fatalf("after device.set: %v", seen)
		}
		if m["id"] == float64(21) && m["result"] == nil {
			t.Fatalf("device.set: %v", m)
		}
		if m["method"] == "event" {
			seen[m["params"].(map[string]any)["type"].(string)]++
		}
	}
}
