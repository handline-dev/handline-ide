-- task_turns: the stack of turns above a task's owner (docs/design/turns.md), bottom first by pos. The top turn's agent acts on the task
-- now (db.ActingSQL); a task with none is acted on by its owner. on_pass "done" with evidence closes the task when that turn passes.
CREATE TABLE IF NOT EXISTS task_turns (
  task_id   INTEGER NOT NULL REFERENCES tasks(id),
  pos       INTEGER NOT NULL,
  agent     TEXT NOT NULL,
  ask       TEXT NOT NULL DEFAULT '',
  pushed_by TEXT NOT NULL,
  since     INTEGER NOT NULL,
  on_pass   TEXT,
  evidence  TEXT,
  PRIMARY KEY (task_id, pos)
) WITHOUT ROWID;

INSERT OR IGNORE INTO table_changes(name) VALUES ('task_turns');

CREATE TRIGGER IF NOT EXISTS tc_task_turns_i AFTER INSERT ON task_turns
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_turns'; END;

CREATE TRIGGER IF NOT EXISTS tc_task_turns_u AFTER UPDATE ON task_turns
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_turns'; END;

CREATE TRIGGER IF NOT EXISTS tc_task_turns_d AFTER DELETE ON task_turns
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_turns'; END;
