-- One agent's load (agentload.Of, read at each session start and for each agent on Devices) reads its own minutes.
CREATE INDEX IF NOT EXISTS load_minutes_agent ON load_minutes(agent, minute);
