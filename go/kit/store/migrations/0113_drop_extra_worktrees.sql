-- extra_worktrees goes: an agent works in one desk of the worktree pool at a time; a folder kept for it is a pin (worktree_folders.pinned_to).
ALTER TABLE agents DROP COLUMN extra_worktrees;
ALTER TABLE agent_state DROP COLUMN extra_worktrees;
