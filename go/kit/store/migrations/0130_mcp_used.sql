-- mcp_used: when an agent (its key) last called a tool of an MCP server (mcp__<server>__*), by the server's name; placement counts a
-- server of the shared MCP config as one an agent needs only when it called it within policy.HeavyKeep.
CREATE TABLE mcp_used (
  agent TEXT NOT NULL,
  server TEXT NOT NULL,
  at INTEGER NOT NULL,
  PRIMARY KEY (agent, server)
) WITHOUT ROWID;
