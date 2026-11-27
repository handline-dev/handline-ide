-- pr_comments_told: the comments people wrote on a task's pull request that its holder was told, each once (watch.Comment.Key, which
-- names a comment, a review's words or a line comment of a review).
CREATE TABLE pr_comments_told (
  task_id INTEGER NOT NULL REFERENCES tasks(id),
  comment TEXT NOT NULL,
  memo    INTEGER REFERENCES task_memos(id) ON DELETE SET NULL,
  told_at INTEGER NOT NULL,
  PRIMARY KEY (task_id, comment)
) WITHOUT ROWID;
