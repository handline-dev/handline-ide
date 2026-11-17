package chatwrites

import (
	"fmt"
	"strings"
	"testing"

	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/taskboard/msgtest"
)

func (r *rig) state(rid int64) (state string) {
	r.t.Helper()
	if err := r.db.QueryRow("SELECT state FROM requests WHERE id = ?", rid).Scan(&state); err != nil {
		r.t.Fatal(err)
	}
	return state
}

func TestWithdrawTakesBackTheCallersOwnUndeliveredRequestAndSaysWhyNot(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	alice, founder := r.server("alice"), r.server("")
	withdraw := func(by int, rid int64, more, key string) map[string]any {
		t.Helper()
		s := alice
		if by == 0 {
			s = founder
		}
		return r.result(r.one(s, "request.withdraw", fmt.Sprintf(`{"id":%d%s,"request_id":%q}`, rid, more, key)))
	}
	mine := r.request("alice", "bob", "look at this")
	if res := withdraw(0, mine, "", "w1"); res["ok"] != false || !strings.Contains(res["said"].(string), "not yours to withdraw") || r.state(mine) != "queued" {
		t.Fatalf("another's request is refused in the reply: %v", res)
	}
	res := withdraw(1, mine, `,"reason":"asked the wrong one"`, "w2")
	if res["ok"] != true || msgtest.IDs(r.db, res["said"].(string)) != fmt.Sprintf("request #%d to %s withdrawn", mine, addr.Ref(r.k("bob"))) || r.state(mine) != "withdrawn" {
		t.Fatalf("%v %s", res, r.state(mine))
	}
	if r.count("SELECT COUNT(*) FROM events WHERE agent = ? AND kind = 'withdraw' AND payload LIKE '%asked the wrong one%'", r.k("alice")) != 1 {
		t.Fatal("the reason is kept with the withdrawal")
	}
	if res := withdraw(1, mine, "", "w3"); res["ok"] != false || !strings.Contains(res["said"].(string), "already withdrawn") {
		t.Fatalf("%v", res)
	}
	sent := r.request("alice", "bob", "too late")
	dbpkg.MarkDelivered(r.db, sent)
	if res := withdraw(1, sent, "", "w4"); res["ok"] != false || !strings.Contains(res["said"].(string), "too late to withdraw") || r.state(sent) != "queued" {
		t.Fatalf("%v", res)
	}
	if res := withdraw(1, 999999, "", "w5"); res["ok"] != false || res["said"] != "request 999999 does not exist" {
		t.Fatalf("%v", res)
	}
	own := r.request("human", "alice", "never mind")
	if res := withdraw(0, own, "", "w6"); res["ok"] != true || r.state(own) != "withdrawn" {
		t.Fatalf("with the founder's key the request is the founder's own: %v", res)
	}
	for _, bad := range []string{`{"id":"one","request_id":"w7"}`, `{"id":1,"reason":"","request_id":"w8"}`, `{"id":1,"reason":7,"request_id":"w9"}`} {
		e, _ := r.one(alice, "request.withdraw", bad)["error"].(map[string]any)
		if data, _ := e["data"].(map[string]any); data["code"] != "bad_request" {
			t.Fatalf("%s: %v", bad, e)
		}
	}
}

func TestAStrangersRequestIsAnsweredAsAMissingOneByIdOrRef(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	alice := r.server("alice")
	theirs := r.request("bob", "human", "private")
	var ref string
	r.db.QueryRow("SELECT ref FROM requests WHERE id = ?", theirs).Scan(&ref)
	for i, id := range []string{fmt.Sprint(theirs), fmt.Sprintf("%q", ref), fmt.Sprintf("%q", "msg_"+ref[:6])} {
		res := r.result(r.one(alice, "request.withdraw", fmt.Sprintf(`{"id":%s,"request_id":"s%d"}`, id, i)))
		if res["ok"] != false || !strings.HasSuffix(res["said"].(string), " does not exist") || r.state(theirs) != "queued" {
			t.Errorf("%s: %v", id, res)
		}
	}
	if res := r.result(r.one(alice, "request.withdraw", `{"id":999999,"request_id":"s9"}`)); res["said"] != "request 999999 does not exist" {
		t.Errorf("a missing one: %v", res)
	}
}
