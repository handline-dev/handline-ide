package daemon

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/watcher"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/scheduler/procload"
)

// watchedTypes are the event types the test counts, one source change each: a task, an agent view, a task-done event, a request and a
// proposal file.
var watchedTypes = []string{"task.changed", "agent.changed", "task_done", "chat.message", "proposal.raised"}

// watchedRows is how many rows of each type the daemon's stream holds once the watch has had its first look and the changes are made.
func watchedRows(t *testing.T, watch bool) map[string]int {
	t.Helper()
	dir, err := os.MkdirTemp("", "hl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"), Auth: fakeAuth{}, Watch: watch})
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("this environment will not bind a socket: %v", err)
		}
		t.Fatal(err)
	}
	defer d.Close()
	if watch { // the last detector's first look sets its cursors
		for i := 0; i < 200; i++ {
			var n int
			if d.db.QueryRow("SELECT COUNT(*) FROM cursors WHERE name = 'stream:chat_prompts'").Scan(&n); n > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	} else {
		time.Sleep(3 * watcher.Every)
	}
	teamtest.Seed(t, dir, "bob")
	bob := teamtest.Key(t, dir, "bob")
	for _, q := range [][]any{
		{"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 't', ?, ?, 1)", bob, actor.OwnerKey},
		{`INSERT INTO agent_view(agent, view, updated) VALUES(?, '{"name":"bob","state":"idle"}', 0)`, bob},
		{`INSERT INTO events(ts, agent, kind, payload) VALUES(1, ?, 'mark_task_done', '{"task_id": 1}')`, bob},
		{"INSERT INTO requests(id, ts, requester, target, body, state, kind) VALUES(1, 5, ?, ?, 'hi', 'queued', 'say')", actor.OwnerKey, bob},
	} {
		if _, err := d.db.Exec(q[0].(string), q[1:]...); err != nil {
			t.Fatal(err)
		}
	}
	// a proposal file: the store is files, so no database commit tells of it
	os.MkdirAll(filepath.Join(dir, "proposals"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "proposals", "0000000a.yaml"),
		[]byte("id: \"0000000a\"\nkind: retire\nagent: old\nrequested_by: boss\nwhy: idle\ncreated: 5\nstate: proposed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(4 * watcher.Every)
	out := map[string]int{}
	for _, typ := range watchedTypes {
		var n int
		if err := d.db.QueryRow("SELECT COUNT(*) FROM stream_log WHERE type = ?", typ).Scan(&n); err != nil && err != sql.ErrNoRows {
			t.Fatal(err)
		}
		out[typ] = n
	}
	return out
}

func TestTheWatcherIsOffUnlessAskedForAndThenOneChangeIsOneRow(t *testing.T) {
	for typ, n := range watchedRows(t, false) {
		if n != 0 {
			t.Errorf("the watcher was off and wrote %d %s rows", n, typ)
		}
	}
	for typ, n := range watchedRows(t, true) {
		if n != 1 {
			t.Errorf("one change wrote %d %s rows", n, typ)
		}
	}
}

// A tracker the caller made is the one the daemon samples and serves, with the database and the panes set on it.
func TestTheDaemonRunsTheLoadTrackerItIsGiven(t *testing.T) {
	dir, err := os.MkdirTemp("", "hl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	tr := &procload.Tracker{Home: dir}
	d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"), Auth: fakeAuth{}, Watch: true, Load: tr})
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("this environment will not bind a socket: %v", err)
		}
		t.Fatal(err)
	}
	defer d.Close()
	if d.load != tr || tr.DB == nil || tr.Panes == nil {
		t.Fatalf("daemon's tracker %p, given %p, db %v", d.load, tr, tr.DB != nil)
	}
}
