-- tasks.turn_origin (added in store.go, addTurnOrigin): the founder's message a task became a request from in the turn that brought it
-- (db.turnRequest). Tasks made before have none.
CREATE INDEX IF NOT EXISTS tasks_turn_origin ON tasks(turn_origin) WHERE turn_origin IS NOT NULL;
