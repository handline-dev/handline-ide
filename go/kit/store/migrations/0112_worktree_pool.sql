-- worktree_folders: the desks of a project, the worktree folders agents take when they start and give back when their session closes.
-- holder is the agent working in it now (NULL: free); last_agent and last_used who had it last and when; pinned_to an agent key or
-- 'founder', and nobody else takes it. agent_state.worktree stays the folder an agent holds or last held.
CREATE TABLE worktree_folders (
  path TEXT PRIMARY KEY,
  repo TEXT NOT NULL,
  holder TEXT,
  last_agent TEXT,
  last_used INTEGER NOT NULL DEFAULT 0,
  pinned_to TEXT,
  untracked_told INTEGER NOT NULL DEFAULT 0 -- the give back held for untracked files was told to the holder in this hold
);
CREATE INDEX worktree_folders_repo ON worktree_folders(repo, last_used);
-- branch: the agent's own branch, checked out in whichever desk it takes.
ALTER TABLE agent_state ADD COLUMN branch TEXT;
