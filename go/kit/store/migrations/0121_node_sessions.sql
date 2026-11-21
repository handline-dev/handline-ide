-- node_sessions: the agent sessions the master opened on a runner (go/internal/nodes): a runner forwards an agent's call only for an
-- agent whose session is on it.
CREATE TABLE node_sessions (
  agent TEXT PRIMARY KEY,
  node TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  handle TEXT NOT NULL,
  opened_at INTEGER NOT NULL
);
