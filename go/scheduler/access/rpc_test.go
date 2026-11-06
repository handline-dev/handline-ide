package access

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/protocol/rpc"
)

type founder struct{}

func (founder) AgentOf(int) string         { return "" }
func (founder) FounderKeyOK(k string) bool { return k == "founder" }
func (founder) SessionsReadable() error    { return nil }

// call serves one founder connection and returns the replies to the calls given, each {method, params}.
func call(t *testing.T, srv *rpc.Server, calls ...[2]string) []map[string]any {
	t.Helper()
	cl, sv := net.Pipe()
	done := make(chan struct{})
	go func() { srv.Serve(rpc.NewLineTransport(sv), 0); sv.Close(); close(done) }()
	defer func() { cl.Close(); <-done }()
	rd := bufio.NewReader(cl)
	cl.SetDeadline(time.Now().Add(10 * time.Second))
	lines := []string{`{"jsonrpc":"2.0","id":0,"method":"hello","params":{"client":"t","protocol":1,"key":"founder"}}`}
	for i, c := range calls {
		lines = append(lines, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, i+1, c[0], c[1]))
	}
	var out []map[string]any
	for _, line := range lines {
		cl.Write([]byte(line + "\n"))
		b, err := rd.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal(b, &m)
		out = append(out, m)
	}
	return out[1:]
}

// A path is absolute, on one line and as written clean; a folder is given to the running session at once, a file from the next one;
// none before a session started; a retried grant is answered from the record, given and announced once.
func TestGrantPathsAndRetry(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "agents", "a"), 0o755)
	os.WriteFile(filepath.Join(home, "agents", "a", "agent.yaml"), []byte("vendor: claude\n"), 0o644)
	srv := rpc.NewServer(founder{}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	srv.DB = storetest.Open(t)
	var events []map[string]any
	var lived []string
	Register(srv, home, func(topic, typ string, data map[string]any) error {
		events = append(events, data)
		return nil
	}, func(agent, dir string) bool { lived = append(lived, dir); return true })
	dir := t.TempDir()
	grant := func(path, rid string) [2]string {
		b, _ := json.Marshal(path)
		return [2]string{"access.grant", fmt.Sprintf(`{"agent":"a","path":%s,"scope":"always","request_id":%q}`, b, rid)}
	}
	var calls [][2]string
	bad := []string{dir + "/../x", dir + "/", dir + "//x", dir + "\nx", "x"}
	for i, p := range bad {
		calls = append(calls, grant(p, fmt.Sprint("b", i)))
	}
	file := filepath.Join(dir, "f")
	os.WriteFile(file, []byte("x"), 0o600)
	calls = append(calls, grant(dir, "ok"), grant(dir, "ok"), grant(file, "file"))
	if r := call(t, srv, grant(dir, "pre"))[0]["result"].(map[string]any); r["applies_from"] != "next_session" || len(lived) != 0 {
		t.Fatalf("no session started: %v, live %v", r, lived)
	}
	events = nil
	srv.DB.Exec("INSERT INTO session_starts(agent, ts, cause, resumed, text, prompt) VALUES('a', 1, 'wake', 0, '', '')")
	got := call(t, srv, calls...)
	for i, p := range bad {
		e, _ := got[i]["error"].(map[string]any)
		if d, _ := e["data"].(map[string]any); d["code"] != "bad_request" || d["field"] != "path" {
			t.Errorf("%q: %v", p, got[i])
		}
	}
	res, _ := got[len(bad)]["result"].(map[string]any)
	if g, _ := res["grant"].(map[string]any); g["path"] != dir || res["applies_from"] != "now" {
		t.Fatalf("%v", got[len(bad)])
	}
	if f, _ := got[len(bad)+2]["result"].(map[string]any); f["applies_from"] != "next_session" || fmt.Sprint(lived) != fmt.Sprint([]string{dir}) {
		t.Fatalf("a file is not given live: %v, live %v", got[len(bad)+2], lived)
	}
	if len(events) != 2 || fmt.Sprint(events[0]["grants"]) != fmt.Sprint([]map[string]any{res["grant"].(map[string]any)}) {
		t.Fatalf("events %v", events)
	}
}
