-- worktree_folders.untracked_told goes: a give back no longer waits on untracked files, so nothing tells their holder or reads the mark.
ALTER TABLE worktree_folders DROP COLUMN untracked_told;
