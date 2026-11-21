-- agent_heard: when an agent's session last ran a tool (PostToolUse, PostToolUseFailure), one row each, kept in place. A long turn of
-- tool calls is heard, not silent. No table_changes trigger: a write per tool call must wake no watcher.
CREATE TABLE agent_heard (
  agent TEXT PRIMARY KEY,
  at INTEGER NOT NULL
);
