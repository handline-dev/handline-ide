package daemon

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConnectionGetIsTheFoundersAndSaysADaemonWithNoListenerHasNone(t *testing.T) {
	d := start(t, fakeAuth{key: "k"}, "")
	if err := os.WriteFile(filepath.Join(d.cfg.Home, "config.yaml"), []byte("dev_mode: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := dial(t, d)
	c.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)
	res, _ := c.raw(t, `{"jsonrpc":"2.0","id":2,"method":"connection.get","params":{}}`)["result"].(map[string]any)
	if res["listener"] != nil || res["dev_mode"] != true {
		t.Fatalf("%v", res)
	}
	agent := dial(t, start(t, fakeAuth{agent: "worker"}, ""))
	agent.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1}}`)
	if r := agent.raw(t, `{"jsonrpc":"2.0","id":2,"method":"connection.get","params":{}}`); r["error"] == nil {
		t.Fatalf("an agent was served connection.get: %v", r)
	}
}

func TestConnectionGetNamesTheListenerAndWhetherItIsTLSOrPlain(t *testing.T) {
	d, _ := startListening(t, "")
	c := helloAs(t, d, mustPair(t, d))
	res, _ := c.ask(`{"jsonrpc":"2.0","id":2,"method":"connection.get","params":{}}`)["result"].(map[string]any)
	l, _ := res["listener"].(map[string]any)
	if l == nil || l["address"] != d.NetworkAddr() || l["encryption"] != "tls" || l["fingerprint"] != d.NetworkFingerprint() || l["tailscale_host"] != nil {
		t.Fatalf("%v", res)
	}
	p := startPlain(t, "")
	pl, _ := p.connectionGet(nil, nil)
	if l, _ := pl.(map[string]any)["listener"].(map[string]any); l == nil || l["encryption"] != "none" || l["fingerprint"] != nil {
		t.Fatalf("%v", pl)
	}
}

func TestAnInheritedConnectionWaitsForItsFirstByteBeforeTheLimitApplies(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c := &inherited{Conn: a}
	c.SetReadDeadline(time.Now().Add(-time.Second))
	go b.Write([]byte("x"))
	if n, err := c.Read(make([]byte, 1)); n != 1 || err != nil {
		t.Fatalf("before its first byte a read waits past the deadline: got %d, %v", n, err)
	}
	c.SetReadDeadline(time.Now().Add(-time.Second))
	if _, err := c.Read(make([]byte, 1)); !os.IsTimeout(err) {
		t.Fatalf("after its first byte the deadline holds: got %v", err)
	}
}
