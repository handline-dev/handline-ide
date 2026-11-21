-- agent_state.node: the machine an agent stays on (go/internal/placement), its node id; NULL until its first placement (an agent that
-- ran before is on this machine).
ALTER TABLE agent_state ADD COLUMN node TEXT;
