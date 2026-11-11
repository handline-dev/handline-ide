package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	"github.com/handline/handline/go/data/team/teamtest"
)

// With chat_transcript_rows on, what a running turn writes to its transcript reaches chat.transcript as reply and tool items; off, none.
func TestARunningTurnsTranscriptFillsTheChatOnlyWithTheSetting(t *testing.T) {
	for _, on := range []bool{false, true} {
		dir, err := os.MkdirTemp("", "hl")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		tracker := &turns.Tracker{Log: t.Logf}
		d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"),
			Auth: fakeAuth{key: "k"}, Turns: tracker, Log: t.Logf})
		if err != nil {
			if strings.Contains(err.Error(), "operation not permitted") {
				t.Skipf("this environment will not bind a socket: %v", err)
			}
			t.Fatal(err)
		}
		t.Cleanup(d.Close)
		config := "chat_transcript_rows: false\n"
		if on {
			config = "chat_transcript_rows: true\n"
		}
		teamtest.Seed(t, dir, "alice", "bob")
		bob := teamtest.Key(t, dir, "bob") // the hook and the tracker see the key, as hook.record's resolved agent
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644)
		path := filepath.Join(dir, "s1.jsonl")
		os.WriteFile(path, nil, 0o644)
		tracker.Hook(bob, "UserPromptSubmit", map[string]any{"transcript_path": path, "session_id": "s1"}, time.Now().Unix())
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		f.WriteString(`{"type":"assistant","message":{"content":[{"type":"text","text":"Reading it."},{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/a.go"}}]}}` + "\n" +
			`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu1","content":"package a"}]}}` + "\n")
		f.Close()
		c := dial(t, d)
		c.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)
		kinds := func() map[string]map[string]any {
			r := c.raw(t, `{"jsonrpc":"2.0","id":2,"method":"chat.transcript","params":{"agent":"bob"}}`)
			res, ok := r["result"].(map[string]any)
			if !ok {
				t.Fatalf("chat.transcript: %v", r)
			}
			items, _ := res["items"].([]any)
			out := map[string]map[string]any{}
			for _, it := range items {
				m := it.(map[string]any)
				if k, _ := m["kind"].(string); k == "reply" || k == "tool" {
					out[k] = m
				}
			}
			return out
		}
		// The tool row is told with its call and gets its output when the result line is read: wait for what is checked, not the rows.
		filled := func(g map[string]map[string]any) bool {
			return g["reply"]["text"] == "Reading it." && g["tool"]["output"] == "package a"
		}
		got := kinds()
		for end := time.Now().Add(5 * time.Second); on && !filled(got) && time.Now().Before(end); got = kinds() {
			time.Sleep(50 * time.Millisecond)
		}
		if !on {
			// The watcher tells a line before it reads the line's tool call, so once Doing names the call the first line was told.
			for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
				if dg := tracker.Doing(bob); dg != nil && dg.Text == "Read /a.go" {
					break
				}
			}
			if dg := tracker.Doing(bob); dg == nil || dg.Text != "Read /a.go" {
				t.Fatalf("off: the watcher never read the tool call: %v", dg)
			}
			var n int
			d.db.QueryRow("SELECT count(*) FROM transcript_events").Scan(&n)
			if got = kinds(); len(got) != 0 || n != 0 {
				t.Fatalf("off: %d rows, %v", n, got)
			}
			continue
		}
		if !filled(got) {
			t.Fatalf("on: %v", got)
		}
	}
}
