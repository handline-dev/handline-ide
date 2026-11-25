package daemon

import (
	"testing"
	"time"
)

// redeemAs pairs a device that says it is kind (none when ""), and answers its token and id.
func redeemAs(t *testing.T, d *Daemon, name, kind string) (token, id string) {
	t.Helper()
	code, _, _ := d.dev.Pair(time.Minute)
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
// to itself.
func TestDevicesListKindClientAndCurrent(t *testing.T) {
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

}
