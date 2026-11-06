-- tasks.request (added in store.go, addTaskRequest): the founder request a task serves, the number of that request's root task (db.SetTaskRequest).
-- Tasks made before have none.
CREATE INDEX IF NOT EXISTS tasks_request ON tasks(request) WHERE request IS NOT NULL;
