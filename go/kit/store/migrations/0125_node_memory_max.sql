-- nodes.memory_max_gb: the most memory, in GB, the agents' sessions on a lent machine may use before it takes no more (device.set);
-- NULL is the master's agents_memory_max_gb from config.yaml, which the master's own row always uses.
ALTER TABLE nodes ADD COLUMN memory_max_gb INTEGER CHECK (memory_max_gb IS NULL OR memory_max_gb >= 1);
