-- agents.move_background_wait_s: how long this agent's usage move waits on its background work, over the global setting.
ALTER TABLE agents ADD COLUMN move_background_wait_s INTEGER;
