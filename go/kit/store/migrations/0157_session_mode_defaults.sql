-- workspaces.session_mode: how its agents' sessions run unless an agent says (terminal or chat).
-- agent_state.terminal_hold: Open terminal on a Chat agent; 1 asked (its next session runs in a terminal), 2 that session runs now.
ALTER TABLE workspaces ADD COLUMN session_mode TEXT NOT NULL DEFAULT 'terminal';
ALTER TABLE agent_state ADD COLUMN terminal_hold INTEGER NOT NULL DEFAULT 0;
