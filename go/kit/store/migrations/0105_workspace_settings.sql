-- workspace_settings: the kept rules (docs/design/people-workspaces.md "Workspace rules"). workspace is a workspaces id, or 0 for the
-- machine default, which a workspace's own row beats. value is JSON; set_by is the key of the person who set it.
CREATE TABLE workspace_settings (
  workspace INTEGER NOT NULL,
  key TEXT NOT NULL,
  value TEXT NOT NULL,
  set_by TEXT NOT NULL,
  set_at INTEGER NOT NULL,
  PRIMARY KEY (workspace, key)
);
