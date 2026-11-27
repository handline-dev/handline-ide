package taskwrites

import (
	"database/sql"
	"strings"
	"testing"
)

// prLink reads task 9's pull request as "<repository>#<number>", "#" for none.
func prLink(t *testing.T, sdb *sql.DB) string {
	t.Helper()
	var pr, repo sql.NullString
	if err := sdb.QueryRow("SELECT pr, pr_repo FROM tasks WHERE id = 9").Scan(&pr, &repo); err != nil {
		t.Fatal(err)
	}
	return repo.String + "#" + pr.String
}

// A wait on a pull request names it as the task's, repository and number, in the write that opens the gate, with the change in the
// task's history; a later wait on another one moves the link, and a wait on an issue links nothing.
func TestTaskWaitOnAPullRequestNamesItAsTheTasks(t *testing.T) {
	loggedIn(t)
	sdb, _, run := reportEnv(t, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(9, 'ship', 'w', 'boss', 1, 'boss')")
	if r := run("w", "task.wait", "w1", `,"task":9,"watch":{"vendor":"github","target":"o/r#12","until":"done"}`); r["error"] != nil || prLink(t, sdb) != "#" {
		t.Fatalf("an issue is no pull request: %v %q", r["error"], prLink(t, sdb))
	}
	if r := run("w", "task.wait", "w2", `,"task":9,"watch":{"vendor":"github","target":"o/r#7"}`); r["error"] != nil || prLink(t, sdb) != "o/r#7" {
		t.Fatalf("%v %q", r["error"], prLink(t, sdb))
	}
	if r := run("w", "task.wait", "w3", `,"task":9,"watch":{"vendor":"github","target":"o/r#7","until":"closed"}`); r["error"] != nil {
		t.Fatalf("%v", r["error"])
	}
	r := run("w", "task.wait", "w4", `,"task":9,"watch":{"vendor":"github","target":"other/fork#8","until":"landed"}`)
	if r["error"] != nil || prLink(t, sdb) != "other/fork#8" || !strings.Contains(said(t, r), "watching PR #8 to land") {
		t.Fatalf("%v %q", r, prLink(t, sdb))
	}
	rows, err := sdb.Query("SELECT json_extract(payload, '$.pr.old'), json_extract(payload, '$.pr.new') FROM events WHERE kind = 'edit_task' AND " +
		"json_extract(payload, '$.task_id') = 9 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var history []string
	for rows.Next() {
		var was, now string
		if err := rows.Scan(&was, &now); err != nil {
			t.Fatal(err)
		}
		history = append(history, was+" -> "+now)
	}
	if got := strings.Join(history, "; "); got != " -> o/r#7; o/r#7 -> other/fork#8" {
		t.Fatalf("history: %s", got)
	}
}

// task.edit pr names a pull request by number, in a repository when pr_repo says which: a bare number forgets the repository the task
// had, since it may be another's, and the same pull request again is refused as nothing to edit.
func TestTaskEditNamesAPullRequestWithItsRepository(t *testing.T) {
	sdb, _, run := reportEnv(t, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(9, 'ship', 'w', 'boss', 1, 'boss')")
	r := run("w", "task.edit", "e1", `,"task":9,"pr":7,"pr_repo":"o/r"`)
	if r["error"] != nil || prLink(t, sdb) != "o/r#7" || !strings.Contains(said(t, r), "pr '' -> 'o/r#7'") {
		t.Fatalf("%v %q", r, prLink(t, sdb))
	}
	if e := errText(run("w", "task.edit", "e2", `,"task":9,"pr":7,"pr_repo":"o/r"`)); !strings.Contains(e, "already names PR #7") {
		t.Fatal(e)
	}
	if e := errText(run("w", "task.edit", "e3", `,"task":9,"pr":7`)); !strings.Contains(e, "already names PR #7") {
		t.Fatalf("the same number with no repository is the same pull request: %q", e)
	}
	if r := run("w", "task.edit", "e4", `,"task":9,"pr":8`); r["error"] != nil || prLink(t, sdb) != "#8" {
		t.Fatalf("%v %q", r["error"], prLink(t, sdb))
	}
	// a repository on another host is host/path, and its merge request is written as that host writes one
	if r := run("w", "task.edit", "e5", `,"task":9,"pr":5,"pr_repo":"git.example.com/team/app"`); r["error"] != nil ||
		!strings.Contains(said(t, r), "-> 'git.example.com/team/app!5'") {
		t.Fatalf("%v", r)
	}
	for i, c := range []string{`,"task":9,"pr_repo":"o/r"`, `,"task":9,"pr":9,"pr_repo":"not a repo"`, `,"task":9,"pr":9,"pr_repo":7`,
		`,"task":9,"pr":9,"pr_repo":"../.."`, `,"task":9,"pr":9,"pr_repo":"o/.."`, `,"task":9,"pr":9,"pr_repo":"./r"`,
		`,"task":9,"pr":9,"pr_repo":"git.example.com/../app"`, `,"task":9,"pr":9,"pr_repo":"github.com/o/r"`} {
		if e := errText(run("w", "task.edit", "bad"+string(rune('a'+i)), c)); !strings.Contains(e, "pr_repo is the repository of pr as owner/name") {
			t.Errorf("%s: %q", c, e)
		}
	}
}
