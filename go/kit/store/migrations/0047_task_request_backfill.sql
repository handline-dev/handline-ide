-- tasks.request for the tasks open now that were made before it (0043), once, by db.SetTaskRequest's rules that the rows still show: a
-- founder's task with no parent is its own request, a task under one by parent serves the request at the top, and a later task of none
-- that a task of a request waits on (an audit) serves that request. Closed tasks and every other column are left as they are.
UPDATE tasks SET request = id WHERE request IS NULL AND done_evidence IS NULL AND discarded_by IS NULL AND parent_task IS NULL
  AND (COALESCE(reporter, opened_by) = 'human' OR from_request IS NOT NULL OR turn_origin IS NOT NULL);
WITH RECURSIVE up(id, at) AS (
  SELECT id, parent_task FROM tasks WHERE request IS NULL AND done_evidence IS NULL AND discarded_by IS NULL AND parent_task IS NOT NULL
  UNION SELECT up.id, t.parent_task FROM up JOIN tasks t ON t.id = up.at WHERE t.request IS NULL AND t.parent_task IS NOT NULL
), top(id, request) AS (
  SELECT up.id, COALESCE(t.request, CASE WHEN COALESCE(t.reporter, t.opened_by) = 'human' OR t.from_request IS NOT NULL OR t.turn_origin IS NOT NULL
    THEN t.id END) FROM up JOIN tasks t ON t.id = up.at WHERE t.request IS NOT NULL OR t.parent_task IS NULL
)
UPDATE tasks SET request = (SELECT request FROM top WHERE top.id = tasks.id)
  WHERE id IN (SELECT id FROM top WHERE request IS NOT NULL);
UPDATE tasks SET request = (SELECT w.request FROM task_blocks b JOIN tasks w ON w.id = b.task_id
    WHERE b.blocks_on = tasks.id AND w.id < tasks.id AND w.request IS NOT NULL ORDER BY b.created_at, w.id LIMIT 1)
  WHERE request IS NULL AND done_evidence IS NULL AND discarded_by IS NULL AND EXISTS (SELECT 1 FROM task_blocks b JOIN tasks w ON w.id = b.task_id
    WHERE b.blocks_on = tasks.id AND w.id < tasks.id AND w.request IS NOT NULL);
