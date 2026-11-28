-- agents.restart_reclaimed: this agent's own switch for starting again elsewhere once reclaimed, over the global one.
ALTER TABLE agents ADD COLUMN restart_reclaimed INTEGER;
