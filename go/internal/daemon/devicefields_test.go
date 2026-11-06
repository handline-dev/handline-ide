package daemon

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// redeemAs pairs a device that says it is kind (none when ""), and answers its token and id.
func redeemAs(t *testing.T, d *Daemon, name, kind string) (token, id string) {
	t.Helper()
	code, _, _, _ := d.dev.Pair(time.Minute)
	k := ""
	if kind != "" {
		k = `,"kind":"` + kind + `"`
	}
	r := dialNet(t, d).ask(`{"jsonrpc":"2.0","id":1,"method":"device.redeem","params":{"code":"` + code + `","name":"` + name + `"` + k + `,"request_id":"r-` + name + `"}}`)
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("redeem: %v", r)
	}
	return res["token"].(string), res["device"].(string)
}

// A device lists as the kind it paired as (a kind this daemon does not know is none), with the client its hello named, and as current
// to itself; the founder renames it, and the devices topic hears it.
func TestDevicesListKindClientAndCurrentAndARenameIsTold(t *testing.T) {
	d := startStream(t)
	phone, phoneID := redeemAs(t, d, "phone", "ios")
	_, fridgeID := redeemAs(t, d, "fridge", "fridge")
	c := dialNet(t, d)
	if r := c.ask(`{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"app/1.4.0","protocol":1,"token":"` + phone + `"}}`); r["error"] != nil {
		t.Fatal(r["error"])
	}
	list := c.ask(`{"jsonrpc":"2.0","id":2,"method":"devices.list","params":{}}`)["result"].(map[string]any)["devices"].([]any)
	byID := map[string]map[string]any{}
	for _, x := range list {
		r := x.(map[string]any)
		byID[r["id"].(string)] = r
	}
	if r := byID[phoneID]; r["kind"] != "ios" || r["client"] != "app/1.4.0" || r["current"] != true {
		t.Fatalf("phone %v", r)
	}
	if r := byID[fridgeID]; r["kind"] != nil || r["client"] != nil || r["current"] != false {
		t.Fatalf("fridge %v", r)
	}

	f := readAll(t, pipeTo(t, d, "k"), 2, 3, 4, 5, 6, 7)
	f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"subscribe","params":{"topics":["devices"]}}`)
	for i, name := range []string{"  ", strings.Repeat("한", 65)} { // empty once trimmed; over 64 characters
		id := float64(3 + i)
		line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"device.rename","params":{"device":%q,"name":%q,"request_id":"n%d"}}`, 3+i, phoneID, name, 3+i)
		if _, code := errOf(t, f.ask(id, line)); code != "bad_request" {
			t.Fatalf("name %q: %s", name, code)
		}
	}
	if _, code := errOf(t, f.ask(5, `{"jsonrpc":"2.0","id":5,"method":"device.rename","params":{"device":"dev_none","name":"x","request_id":"n5"}}`)); code != "not_found" {
		t.Fatalf("an unpaired device: %s", code)
	}
	long := strings.Repeat("한", 64) // 64 characters, 192 bytes: the limit is in characters
	if r := f.ask(7, fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"device.rename","params":{"device":%q,"name":%q,"request_id":"n7"}}`, phoneID, long)); r["error"] != nil {
		t.Fatalf("a 64-character name: %v", r["error"])
	}
	if ev := <-f.devices; ev["data"].(map[string]any)["name"] != long {
		t.Fatalf("%v", ev)
	}
	if r := f.ask(6, `{"jsonrpc":"2.0","id":6,"method":"device.rename","params":{"device":"`+phoneID+`","name":"  My phone ","request_id":"n6"}}`); r["error"] != nil {
		t.Fatal(r["error"])
	}
	select {
	case ev := <-f.devices:
		if ev["type"] != "device.renamed" {
			t.Fatalf("%v", ev)
		}
		if dev := ev["data"].(map[string]any); dev["id"] != phoneID || dev["name"] != "My phone" || dev["current"] != false || dev["kind"] != "ios" {
			t.Fatalf("%v", dev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no device.renamed")
	}
}
