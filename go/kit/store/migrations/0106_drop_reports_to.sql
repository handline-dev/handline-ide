-- agents.reports_to goes: agents are peers in a workspace, and its lead is only its contact for work from other workspaces.
ALTER TABLE agents DROP COLUMN reports_to;
