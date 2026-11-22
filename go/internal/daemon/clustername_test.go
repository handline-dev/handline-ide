package daemon

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/handline/handline/go/protocol/rpc"
)

// A board with no cluster name gets two random words at its start, kept on the board across a restart; hello and node.status give it with the
// machines; the owner renames it, a member may not, and it cannot be cleared.
func TestAClusterIsNamedAtItsFirstStartAndOnlyTheOwnerRenamesIt(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth = fakeAuth{key: "k", agent: "a"}
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	owner := rpc.Caller{Kind: "person", Key: "p1", Role: "owner"}
	member := rpc.Caller{Kind: "device", Key: "p2", Person: "p2", Role: "member", Info: map[string]any{"device": "kim-phone"}}
	call := func(as rpc.Caller, method string, p map[string]any) (map[string]any, error) {
		got, err := d.local.Dispatch(as, "t", "", method, p)
		m, _ := got.(map[string]any)
		return m, err
	}
	name := func() string {
		r, err := call(owner, "settings.get", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		return r["values"].(map[string]any)["cluster_name"].(string)
	}
	first := name()
	if !regexp.MustCompile(`^[A-Z][a-z]+ [A-Z][a-z]+$`).MatchString(first) {
		t.Fatalf("the name given at start: %q", first)
	}
	r, err := call(owner, "node.status", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if c := r["cluster"].(map[string]any); c["name"] != first || c["machines"] != 1 {
		t.Fatalf("node.status cluster: %v", c)
	}
	caps := dialNet(t, d).ask(`{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"token":"` + mustPair(t, d) +
		`"}}`)["result"].(map[string]any)["capabilities"].(map[string]any)
	if c := caps["cluster"].(map[string]any); c["name"] != first || c["machines"] != float64(1) {
		t.Fatalf("hello cluster: %v", c)
	}

	set := func(as rpc.Caller, v any) error {
		_, err := call(as, "settings.set", map[string]any{"changes": map[string]any{"cluster_name": v}, "request_id": "cn-" + as.Key})
		return err
	}
	if err := set(member, "Kim's"); err == nil {
		t.Fatal("a member renamed the cluster")
	}
	for _, bad := range []any{nil, "  ", strings.Repeat("x", 41), "two\nlines"} {
		if err := set(owner, bad); err == nil {
			t.Fatalf("cluster_name %q was taken", bad)
		}
	}
	if err := set(owner, "  Home Lab "); err != nil {
		t.Fatal(err)
	}
	if got := name(); got != "Home Lab" {
		t.Fatalf("after the rename: %q", got)
	}
	d.Close()
	if d, err = Start(cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	if got := name(); got != "Home Lab" {
		t.Fatalf("after a restart: %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(cfg.Home, "config.yaml")); strings.Contains(string(b), "cluster_name") {
		t.Fatalf("the name is the board's, not config.yaml's: %s", b)
	}
}
