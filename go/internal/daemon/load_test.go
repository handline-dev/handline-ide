package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/scheduler/sysres"
)

// A daemon that samples serves load.get with its own process in it, and writes the minute rows to its database.
func TestLoadGetIsServedByADaemonThatSamples(t *testing.T) {
	dir, err := os.MkdirTemp("", "hl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"), Auth: fakeAuth{key: "k"},
		Resources: &sysres.Sampler{Home: dir}, Log: t.Logf})
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("this environment will not bind a socket: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	c := dial(t, d)
	c.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)
	var res map[string]any
	for end := time.Now().Add(5 * time.Second); res == nil && time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		res, _ = c.raw(t, `{"jsonrpc":"2.0","id":2,"method":"load.get","params":{"minutes":5}}`)["result"].(map[string]any)
	}
	if res == nil {
		t.Fatalf("load.get stayed null: %v", c.raw(t, `{"jsonrpc":"2.0","id":3,"method":"load.get","params":{}}`))
	}
	if _, ok := res["series"].(map[string]any); !ok || res["every_s"] != 5.0 || res["total"].(map[string]any)["processes"].(float64) < 1 {
		t.Fatalf("%v", res)
	}
}

// The local socket's server is made at Start; it must already hold the sampler, or daemon.status has no resources there.
func TestDaemonStatusOnTheLocalSocketCarriesResources(t *testing.T) {
	dir, err := os.MkdirTemp("", "hl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"), Auth: fakeAuth{key: "k"},
		Resources: &sysres.Sampler{Home: dir}})
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("this environment will not bind a socket: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	c := dial(t, d)
	c.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)
	var res map[string]any
	for end := time.Now().Add(5 * time.Second); res == nil && time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		r, _ := c.raw(t, `{"jsonrpc":"2.0","id":2,"method":"daemon.status","params":{}}`)["result"].(map[string]any)
		res, _ = r["resources"].(map[string]any)
	}
	if res == nil || res["machine"] == nil {
		t.Fatalf("daemon.status had no resources on the local socket: %v", res)
	}
}
