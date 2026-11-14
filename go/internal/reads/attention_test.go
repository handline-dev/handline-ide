package reads

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// RequestRow.attention is the first of overdue, bounced, waiting and quiet that holds in the request's tree, at its threshold and not
// below it; null once the request is done or when nothing holds.
func TestARequestNeedsAttentionByTheFirstFactOfItsTree(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	at := func(n int64) string { return itoa(now - n) }
	// T1 the request, made age seconds ago with T2 under it, and T3 outside it; cols and vals are more of T2's columns.
	tree := func(age int64, cols string, vals ...string) []string {
		v := ""
		for _, x := range vals {
			v += ", " + x
		}
		return []string{
			"INSERT INTO tasks(id, title, owner, opened_by, created_at, request) VALUES(1, 'ask', 'lead', 'p1', " + at(age) + ", 1), (3, 'ops', 'ops', 'ops', " + at(10) + ", NULL)",
			"INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at, request" + cols + ") VALUES(2, 'build', 'dev', 'lead', 1, " + at(age) + ", 1" + v + ")",
		}
	}
	child := func(cols string, vals ...string) []string { return tree(100, cols, vals...) }
	plain := child("")
	reopenOf := func(task, n int, was string) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf(`INSERT INTO events(ts, agent, kind, payload) VALUES(%s, 'lead', 'reopen_task', '{"task_id": %d, "was": "%s"}')`, at(50), task, was))
		}
		return out
	}
	reopen := func(n int, was string) []string { return reopenOf(2, n, was) }
	hold := func(on string, age int64) string {
		return fmt.Sprintf(`INSERT INTO meta(name, value, updated) VALUES('taskhold:2', '{"on": "%s", "since": %d, "why": "busy"}', 1)`, on, now-age)
	}
	join := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	for _, c := range []struct {
		name   string
		sql    []string
		config string
		want   string
	}{
		{"nothing", plain, "", "<nil>"},
		{"quiet at its threshold", tree(7200, ""), "", "quiet 1 no recent activity"},
		{"not quiet below it", tree(7100, ""), "", "<nil>"},
		{"quiet by the setting", plain, "attention_quiet_s: 60\n", "quiet 1 no recent activity"},
		{"overdue", child(", timeout_at", at(60)), "", "overdue 2 T2 past its time box"},
		{"overdue first", join(child(", timeout_at", at(60)), reopen(3, "done"), []string{hold("full", 9000)}), "", "overdue 2 T2 past its time box"},
		{"a gate is never overdue", []string{tree(100, "")[0], "INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at, request, timeout_at) VALUES(2, 'watch', 'gate', 'lead', 1, " + at(100) + ", 1, " + at(60) + ")"}, "", "<nil>"},
		{"a task that waits on an open child is not overdue", join(child(", timeout_at", at(60)),
			[]string{"INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at, request) VALUES(4, 'part', 'dev', 'dev', 2, " + at(100) + ", 1)"}), "", "<nil>"},
		{"not quiet while a clock runs", tree(9000, ", timeout_at", itoa(now+60)), "", "<nil>"},
		{"a clock not yet out", child(", timeout_at", itoa(now+60)), "", "<nil>"},
		{"a done task is not overdue", child(", timeout_at, done_evidence, done_at", at(60), "'ok'", at(30)), "", "<nil>"},
		{"sent back twice", join(plain, reopen(2, "done")), "", "bounced 2 T2 sent back 2 times"},
		{"a done task sent back is past", join(child(", done_evidence, done_at", "'ok'", at(30)), reopen(2, "done")), "", "<nil>"},
		{"sent back once", join(plain, reopen(1, "done")), "", "<nil>"},
		{"a restore is no send-back", join(plain, reopen(1, "done"), reopen(1, "discarded")), "", "<nil>"},
		{"sent back by the setting", join(plain, reopen(1, "done")), "attention_sent_back: 1\n", "bounced 2 T2 sent back 1 times"},
		{"the most sent back", join(plain, reopen(2, "done"), reopenOf(4, 3, "done"),
			[]string{"INSERT INTO tasks(id, title, owner, opened_by, parent_task, created_at, request) VALUES(4, 'part', 'dev', 'lead', 1, " + at(100) + ", 1)"}), "", "bounced 4 T4 sent back 3 times"},
		{"bounced before waiting", join(plain, reopen(2, "done"), []string{hold("full", 9000)}), "", "bounced 2 T2 sent back 2 times"},
		{"waiting on full", join(plain, []string{hold("full", 3600)}), "", "waiting 2 T2 waiting on full"},
		{"waiting on wall", join(plain, []string{hold("wall", 3600)}), "", "waiting 2 T2 waiting on wall"},
		{"waiting on owner", join(plain, []string{hold("owner", 3600)}), "", "waiting 2 T2 waiting on owner"},
		{"waiting before quiet", join(tree(9000, ""), []string{hold("owner", 9000)}), "", "waiting 2 T2 waiting on owner"},
		{"a hold with no since", join(plain, []string{`INSERT INTO meta(name, value, updated) VALUES('taskhold:2', '{"on": "full", "why": "busy"}', 1)`}), "", "<nil>"},
		{"not waiting below it", join(plain, []string{hold("full", 3500)}), "", "<nil>"},
		{"waiting by the setting", join(plain, []string{hold("full", 120)}), "attention_wait_s: 60\n", "waiting 2 T2 waiting on full"},
		{"a host wait does not count", join(plain, []string{hold("host", 9000)}), "", "<nil>"},
		{"behind an open blocker", join(plain, []string{hold("full", 9000), "INSERT INTO task_blocks(task_id, blocks_on, created_at) VALUES(2, 3, 1)"}), "", "<nil>"},
		{"a card out", join(plain, []string{hold("full", 9000),
			"INSERT INTO requests(id, ref, ts, requester, target, body, state, kind) VALUES(9, 'aaaaaaaaaaa9', 1, 'dev', 'p1', 'ok?', 'queued', 'say')",
			"INSERT INTO task_requests(task_id, request_id) VALUES(2, 9)"}), "", "<nil>"},
		{"a done request", []string{"INSERT INTO tasks(id, title, owner, opened_by, created_at, request, done_evidence, done_at) VALUES(1, 'ask', 'lead', 'p1', " + at(9000) + ", 1, 'ok', " + at(9000) + ")"}, "", "<nil>"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := testEnv(t)
			for _, q := range c.sql {
				run(t, e, q)
			}
			if c.config != "" {
				if err := os.WriteFile(filepath.Join(e.Home, "config.yaml"), []byte(c.config), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			rows, err := e.RequestRows(now)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("one request: %v", rows)
			}
			got := "<nil>"
			if a, ok := rows[0]["attention"].(map[string]any); ok {
				got = fmt.Sprint(a["kind"], " ", a["task"], " ", a["text"])
			} else if rows[0]["attention"] != nil {
				t.Fatalf("attention %#v", rows[0]["attention"])
			}
			if got != c.want {
				t.Errorf("attention %q, want %q", got, c.want)
			}
		})
	}
}
