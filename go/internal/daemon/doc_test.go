package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// switchAuth is fakeAuth whose socket peer is the agent it holds at hello: "" is the founder, who holds the key.
type switchAuth struct{ agent atomic.Value }

func (a *switchAuth) AgentOf(int) string           { s, _ := a.agent.Load().(string); return s }
func (a *switchAuth) SessionsReadable() error      { return nil }
func (a *switchAuth) FounderKeyOK(key string) bool { return key == "k" }

// The shared docs are one set for the whole daemon: a file opened over the socket and over the network is one doc, and an agent's
// PostToolUse hook names it on that doc's disk update.
func TestADocIsOneOverBothDoorsAndTheAgentsHookNamesItsDiskUpdate(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	short, err := os.MkdirTemp("", "hl") // a socket path has a length limit the test's own dir passes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(short) })
	cfg.SocketFile = filepath.Join(short, "d.sock")
	auth := &switchAuth{}
	cfg.Auth = auth
	d, err := Start(cfg)
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("this environment will not bind a socket: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	wt, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec("INSERT INTO agent_state(agent, state, updated, worktree) VALUES('bob', 'IDLE', 0, ?)", "repo::"+wt); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(wt, "a.md")
	if err := os.WriteFile(file, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const open = `{"jsonrpc":"2.0","id":2,"method":"doc.open","params":{"agent":"bob","path":"a.md"}}`
	token, _ := pair(t, d, "web")
	web := dialNet(t, d)
	if r := web.ask(`{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"token":"` + token + `"}}`); r["error"] != nil {
		t.Fatal(r["error"])
	}
	r := web.ask(open)
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("doc.open over the network: %v", r)
	}
	founder := dial(t, d)
	founder.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)
	r = founder.raw(t, open)
	if got, _ := r["result"].(map[string]any); got == nil || got["doc"] != res["doc"] {
		t.Fatalf("over the socket %v, over the network doc %v: two docs of one file", r, res["doc"])
	}

	if err := os.WriteFile(file, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	auth.agent.Store("bob")
	agent := dial(t, d)
	agent.raw(t, hello)
	edit := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"hook.record","params":{"agent":"bob","kind":"PostToolUse",`+
		`"data":{"tool_name":"Edit","tool_input":{"file_path":%q}},"at":%d,"request_id":"e1"}}`, file, time.Now().Unix())
	if r := agent.raw(t, edit); r["error"] != nil {
		t.Fatal(r["error"])
	}
	for end := time.Now().Add(5 * time.Second); ; {
		if time.Now().After(end) {
			t.Fatal("no doc_changed over the network")
		}
		m := web.read()
		if m["method"] != "doc_changed" {
			continue
		}
		u := m["params"].(map[string]any)["updates"].([]any)[0].(map[string]any)
		if u["client"] != "disk" || u["by"] != "bob" {
			t.Fatalf("the disk update %v does not name bob", u)
		}
		return
	}
}
