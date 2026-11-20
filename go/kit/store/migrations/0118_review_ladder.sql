-- review_levels per kind of work: a row is one agent's level at one kind (contract, screen, daemon, docs), set while the agent ran model.
ALTER TABLE review_levels ADD COLUMN work TEXT NOT NULL DEFAULT '';
ALTER TABLE review_levels ADD COLUMN model TEXT NOT NULL DEFAULT '';
DROP INDEX review_levels_agent;
CREATE INDEX review_levels_agent ON review_levels(agent, work, id);
