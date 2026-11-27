package store

import "testing"

// The comment a pull request comment was told in, and the cursor of a comment's unread notice, keep their rows under the new names.
func TestTheToldCommentAndItsLeadCursorSayComment(t *testing.T) {
	db := atVersion(t, 154)
	for _, q := range []string{
		"PRAGMA foreign_keys = OFF",
		"INSERT INTO pr_comments_told(task_id, comment, memo, told_at) VALUES (1, 'c1', 7, 1)",
		"INSERT INTO cursors(name, row_id, updated) VALUES ('memo_lead:42', 1, 1), ('memo_leader', 3, 1), ('other', 2, 1)",
		step(t, "comment_told_names"),
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	var told int64
	var names string
	db.QueryRow("SELECT task_comment FROM pr_comments_told WHERE comment = 'c1'").Scan(&told)
	db.QueryRow("SELECT group_concat(name || '=' || row_id, ',') FROM (SELECT * FROM cursors ORDER BY name)").Scan(&names)
	if told != 7 || names != "comment_lead:42=1,memo_leader=3,other=2" {
		t.Fatalf("told in %d; cursors %q", told, names)
	}
}
