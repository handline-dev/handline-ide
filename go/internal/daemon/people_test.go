package daemon

import (
	"testing"
	"time"
)

func rpcAs(c *wsClient, method, params string) map[string]any {
	return c.ask(`{"jsonrpc":"2.0","id":9,"method":"` + method + `","params":` + params + `}`)
}

// The owner invites Kim and pairs her first device; it acts as a member: it sees only Kim's devices, pairs and lends a machine of Kim's
// own, and is refused what is the owner's. A role change closes her connections, the founder stays an owner, and removing Kim closes
// her connections, ends her token, her machine's lending and any code made for her, and keeps her in people.list as removed.
func TestAnInvitedMemberWorksLendsAndIsRemoved(t *testing.T) {
	d := startNet(t)
	code, _, _ := d.dev.Pair(time.Minute)
	ownerDev := joinAs(t, d, code, "owner-phone", "ios", "")["result"].(map[string]any)
	owner := helloAs(t, d, ownerDev["token"].(string))

	kimKey := rpcAs(owner, "person.invite", `{"display_name":"Kim","request_id":"inv-kim"}`)["result"].(map[string]any)["person"].(string)
	if again := rpcAs(owner, "person.invite", `{"display_name":"Kim","request_id":"inv-kim"}`)["result"].(map[string]any); kimKey == "p1" ||
		again["person"] != kimKey {
		t.Fatalf("invite %s, its retry %v", kimKey, again)
	}
	link := rpcAs(owner, "device.pair", `{"person":"`+kimKey+`","request_id":"kim-link"}`)["result"].(map[string]any)
	kimDev := joinAs(t, d, link["code"].(string), "kim-phone", "ios", "")["result"].(map[string]any)
	kim := helloAs(t, d, kimDev["token"].(string))

	list := rpcAs(kim, "devices.list", `{}`)["result"].(map[string]any)["devices"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["person"] != kimKey || list[0].(map[string]any)["role"] != "member" {
		t.Fatalf("Kim sees: %v", list)
	}
	if all := rpcAs(owner, "devices.list", `{}`)["result"].(map[string]any)["devices"].([]any); len(all) != 2 {
		t.Fatalf("the owner sees: %v", all)
	}
	for _, refused := range []struct{ method, params, gate string }{
		{"person.invite", `{"display_name":"Lee","request_id":"inv-lee"}`, "role"},
		{"device.revoke", `{"device":"` + ownerDev["device"].(string) + `","request_id":"rv"}`, "person"},
		{"device.pair", `{"person":"p1","request_id":"kim-for-p1"}`, "person"},
		{"shell.open", `{"request_id":"sh"}`, "role"},
		{"task.plan_ok", `{"task":1,"request_id":"ok"}`, "role"},
	} {
		if r := rpcAs(kim, refused.method, refused.params); gate(r) != refused.gate {
			t.Fatalf("%s as Kim: %v", refused.method, r)
		}
	}

	mine := rpcAs(kim, "device.pair", `{"request_id":"kim-pair"}`)["result"].(map[string]any)
	box := joinAs(t, d, mine["code"].(string), "kim-box", "server", "n-kim")["result"].(map[string]any)
	if r := set(kim, box["device"].(string), "run"); r["result"] == nil {
		t.Fatalf("Kim cannot run her machine: %v", r)
	}
	if got := lent(t, owner); got["n-kim"] != kimKey+" run" {
		t.Fatalf("lent: %v", got)
	}

	if r := rpcAs(owner, "person.set_role", `{"person":"p1","role":"member","request_id":"demote"}`); gate(r) != "last_owner" {
		t.Fatalf("the founder was made a member: %v", r)
	}
	if r := rpcAs(owner, "person.set_role", `{"person":"`+kimKey+`","role":"owner","request_id":"promote"}`); r["result"] == nil || !kim.closed() {
		t.Fatalf("set_role %v: Kim's connection stayed open on her old role", r)
	}
	kim = helloAs(t, d, kimDev["token"].(string))
	if r := rpcAs(kim, "shell.open", `{"request_id":"sh2"}`); gate(r) == "role" {
		t.Fatalf("Kim made owner is still refused by role: %v", r)
	}
	rpcAs(owner, "person.set_role", `{"person":"`+kimKey+`","role":"member","request_id":"demote-kim"}`)
	kim = helloAs(t, d, kimDev["token"].(string))
	late := rpcAs(owner, "device.pair", `{"person":"`+kimKey+`","request_id":"kim-late"}`)["result"].(map[string]any)["code"].(string)
	if r := rpcAs(owner, "person.remove", `{"person":"`+kimKey+`","request_id":"rm-kim"}`); r["result"] == nil {
		t.Fatalf("remove: %v", r)
	}
	if !kim.closed() {
		t.Fatal("Kim's connection stayed open")
	}
	again := dialNet(t, d)
	if r := again.ask(`{"jsonrpc":"2.0","id":"h","method":"hello","params":{"client":"t","protocol":1,"token":"` + kimDev["token"].(string) + `"}}`); r["result"] != nil {
		t.Fatalf("Kim's token still logs in: %v", r)
	}
	if got := lent(t, owner); got["n-kim"] != "" {
		t.Fatalf("Kim's machine is still lent: %v", got)
	}
	people := rpcAs(owner, "people.list", `{}`)["result"].(map[string]any)["people"].([]any)
	if len(people) != 2 || people[1].(map[string]any)["removed"] != true || people[1].(map[string]any)["display_name"] != "Kim" {
		t.Fatalf("people: %v", people)
	}
	if r := rpcAs(owner, "person.remove", `{"person":"p1","request_id":"rm-me"}`); gate(r) != "last_owner" {
		t.Fatalf("the founder was removed: %v", r)
	}
	if r := joinAs(t, d, late, "kim-tablet", "ios", ""); gate(r) != "pairing_code" {
		t.Fatalf("a code made for Kim paired after she was removed: %v", r)
	}
}
