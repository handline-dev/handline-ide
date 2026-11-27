-- A task comment is named a comment where it is stored: its tables, columns, indexes, change triggers, event kinds and the payload key
-- that names one.
ALTER TABLE task_memos RENAME TO task_comments;
ALTER TABLE memo_tells RENAME TO comment_tells;
ALTER TABLE comment_tells RENAME COLUMN memo_id TO comment_id;
ALTER TABLE requests RENAME COLUMN memo_id TO comment_id;
ALTER TABLE task_turns RENAME COLUMN memo TO comment;

DROP INDEX task_memos_task;
CREATE INDEX task_comments_task ON task_comments(task_id, id);
DROP INDEX requests_memo;
CREATE INDEX requests_comment ON requests(comment_id) WHERE comment_id IS NOT NULL;

DROP TRIGGER tc_task_memos_i;
DROP TRIGGER tc_task_memos_u;
DROP TRIGGER tc_task_memos_d;
UPDATE table_changes SET name = 'task_comments' WHERE name = 'task_memos';
CREATE TRIGGER tc_task_comments_i AFTER INSERT ON task_comments
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_comments'; END;
CREATE TRIGGER tc_task_comments_u AFTER UPDATE ON task_comments
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_comments'; END;
CREATE TRIGGER tc_task_comments_d AFTER DELETE ON task_comments
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_comments'; END;

UPDATE events SET kind = 'comment_' || substr(kind, 6) WHERE kind IN ('memo_add', 'memo_edit', 'memo_delete');
UPDATE events SET payload = json_set(json_remove(payload, '$.memo'), '$.comment', json_extract(payload, '$.memo'))
WHERE kind IN ('comment_add', 'comment_edit', 'comment_delete', 'task_turn_push', 'task_turn_pop', 'task_turn_ask', 'task_ready',
  'task_verdict')
  AND json_valid(payload) AND json_type(payload, '$.memo') IS NOT NULL;

UPDATE meta SET name = 'comment-files:' || substr(name, 12) WHERE name LIKE 'memo-files:%';
