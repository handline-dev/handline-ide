-- A temp is a seat of an agent (go/internal/temps): agents.hired_from is the key of the agent it was hired from, NULL for an agent of its
-- own; agents.hired_for is the task its current hire is for, NULL while it sits idle.
ALTER TABLE agents ADD COLUMN hired_from TEXT;
ALTER TABLE agents ADD COLUMN hired_for INTEGER;
