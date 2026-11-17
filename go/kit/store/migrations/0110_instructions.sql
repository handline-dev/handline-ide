-- instructions: an agent's role and standing context and a workspace's rules, one row per version, newest in force. owner is the agent's
-- or the workspace's key, part is role, base or rules. actor is who wrote it (an agent, a person, "imported", "Handline"),
-- approver who OK'd it ('' for none); the text before is the row before it.
CREATE TABLE instructions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  owner TEXT NOT NULL,
  part TEXT NOT NULL,
  text TEXT NOT NULL,
  actor TEXT NOT NULL,
  approver TEXT NOT NULL DEFAULT '',
  at INTEGER NOT NULL
);
CREATE INDEX instructions_owner ON instructions(owner, part, id);
