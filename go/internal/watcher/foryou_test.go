package watcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/reads"
)

// Each kind of item is told as it joins the count and as it leaves it, with the count after it; an item deferred leaves, and comes back.
func TestForYouTellsEachKindOpeningAndClosingWithTheCount(t *testing.T) {
	d, l := rig(t)
	home := t.TempDir()
	w := &ForYou{Reads: &reads.Env{DB: d, Home: home}}
	told := 0
	scan := func() (out []string) {
		t.Helper()
		if err := w.Scan(d, l.Append); err != nil {
			t.Fatal(err)
		}
		got, err := l.Since(0, []string{"for_you"}, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range got[told:] {
			var data map[string]any
			if err := json.Unmarshal(e.Data, &data); err != nil {
				t.Fatal(err)
			}
			if item, ok := data["item"].(map[string]any); ok {
				out = append(out, fmt.Sprintf("%s %s/%s %v", e.Type, item["kind"], item["id"], data["count"]))
			} else {
				out = append(out, fmt.Sprintf("%s %s/%s %v", e.Type, data["kind"], data["id"], data["count"]))
			}
		}
		told = len(got)
		return out
	}
	step := func(want string, q string, args ...any) {
		t.Helper()
		if q != "" {
			exec(t, d, q, args...)
		}
		if got := scan(); len(got) != 1 || got[0] != want {
			t.Fatalf("want %q, got %q", want, got)
		}
	}
	if got := scan(); len(got) != 0 {
		t.Fatalf("a first look told %v", got)
	}
	now := time.Now().Unix()
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(2, 'other', 'w', 'boss', 1, 'boss')")
	exec(t, d, "INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(7, 2, 'w', '@human?', 1)")

	step("for_you.opened task/1 1", "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'pick', 'p1', 'boss', 1, 'boss')")
	step("for_you.opened comment/7 2", "INSERT INTO requests(id, ref, ts, requester, target, body, state, memo_id, edge_ok) VALUES(70, 'aaaaaaaaaaa7', 1, 'w', 'p1', '@human?', 'queued', 7, 1)")
	step("for_you.opened prompt/fp1 3", "INSERT INTO prompt_blocks(agent, kind, fingerprint, title, panel, detail, first_seen, last_seen) VALUES('bob', 'permission', 'fp1', 't', '[]', '{}', ?, ?)", now, now)
	if err := os.MkdirAll(filepath.Join(home, "proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	hr := "id: \"0000000b\"\nkind: retire\nagent: old\nrequested_by: boss\nwhy: \"idle\"\ncreated: 5\nstate: %s\n"
	put := func(state string) {
		if err := os.WriteFile(filepath.Join(home, "proposals", "0000000b.yaml"), []byte(fmt.Sprintf(hr, state)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("proposed")
	step("for_you.opened proposal/11 4", "")
	step("for_you.opened machine/disk 5", "INSERT INTO cursors(name, row_id, updated) VALUES('disk_full_since', 9, 9)")

	// deferred: it leaves the count; back: it joins again
	later := func(entries string) {
		exec(t, d, "INSERT INTO prefs(person, nav, updated_at) VALUES('p1', ?, 0) ON CONFLICT(person) DO UPDATE SET nav = excluded.nav", `{"later":`+entries+`}`)
	}
	later(fmt.Sprintf(`{"task:1":{"until":%q}}`, time.Unix(now+3600, 0).UTC().Format(time.RFC3339)))
	step("for_you.closed task/1 4", "")
	later(`{}`)
	step("for_you.opened task/1 5", "")
	// its time passing alone brings it back, and the signature says so
	before := ForYouSignature(d)
	later(fmt.Sprintf(`{"task:1":{"until":%q}}`, time.Unix(now+2, 0).UTC().Format(time.RFC3339)))
	step("for_you.closed task/1 4", "")
	later(fmt.Sprintf(`{"task:1":{"until":%q}}`, time.Unix(now-1, 0).UTC().Format(time.RFC3339)))
	step("for_you.opened task/1 5", "")
	if ForYouSignature(d) == before {
		t.Errorf("a deferral that ran out left the signature at %s", before)
	}

	step("for_you.closed task/1 4", "UPDATE tasks SET done_evidence = 'x', done_at = 9 WHERE id = 1")
	step("for_you.closed comment/7 3", "UPDATE requests SET state = 'landed' WHERE id = 70")
	step("for_you.closed prompt/fp1 2", "UPDATE prompt_blocks SET gone_at = 1")
	put("rejected")
	step("for_you.closed proposal/11 1", "")
	step("for_you.closed machine/disk 0", "DELETE FROM cursors WHERE name = 'disk_full_since'")
	if got := scan(); len(got) != 0 {
		t.Fatalf("told again: %v", got)
	}
}

// A task whose question (the founder's turn) is rewritten opens again with the new question, the count unchanged; the same text tells nothing.
func TestForYouTellsARewrittenQuestionAgainOnlyWhenItChanges(t *testing.T) {
	d, l := rig(t)
	w := &ForYou{Reads: &reads.Env{DB: d, Home: t.TempDir()}}
	told := 0
	scan := func() (out []string) {
		t.Helper()
		if err := w.Scan(d, l.Append); err != nil {
			t.Fatal(err)
		}
		got, err := l.Since(0, []string{"for_you"}, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range got[told:] {
			var data map[string]any
			if err := json.Unmarshal(e.Data, &data); err != nil {
				t.Fatal(err)
			}
			out = append(out, fmt.Sprintf("%s %s %v", e.Type, *e.Title, data["count"]))
		}
		told = len(got)
		return out
	}
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'pick', 'w', 'boss', 1, 'boss')")
	scan()
	exec(t, d, "INSERT INTO task_turns(task_id, pos, agent, ask, pushed_by, since) VALUES(1, 1, 'p1', 'A or B?', 'w', 1)")
	if got := scan(); len(got) != 1 || got[0] != "for_you.opened pick 1" {
		t.Fatalf("pushed: %q", got)
	}
	exec(t, d, "UPDATE task_turns SET ask = 'A, B or C?' WHERE task_id = 1")
	if got := scan(); len(got) != 1 || got[0] != "for_you.opened A, B or C? 1" {
		t.Fatalf("rewritten: %q", got)
	}
	if got := scan(); len(got) != 0 {
		t.Fatalf("unchanged told %q", got)
	}
}
