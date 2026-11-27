-- agents.no_silence_reminder: an agent's own switch over the global one.
ALTER TABLE agents ADD COLUMN no_silence_reminder INTEGER;
