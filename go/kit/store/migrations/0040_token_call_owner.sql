-- token_calls.cc1h, agent and task_id (added in store.go, addTokenCallOwner): the part of a call's cache write held for an hour (priced 2x
-- input, not 1.25x), and the agent and the task it held as doing when the call was made, so a task's spend and an agent's pace in dollars
-- are sums over these rows. token_calls_agent_ts finds an agent's call before another, which a task's minutes are measured from. Rows
-- filed before have none: their cache writes are priced as 5-minute ones and they belong to no agent.
CREATE INDEX IF NOT EXISTS token_calls_task ON token_calls(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS token_calls_agent_ts ON token_calls(agent, ts) WHERE agent <> '';
