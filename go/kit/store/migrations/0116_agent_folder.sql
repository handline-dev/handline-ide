-- The folder of its workspace an agent works in; NULL is the workspace's default folder (workspaces.folder).
ALTER TABLE agents ADD COLUMN folder TEXT;
