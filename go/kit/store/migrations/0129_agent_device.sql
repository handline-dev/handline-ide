-- agents.device: the device an agent is pinned to (a node id), which its sessions start on while placement is Manual
-- (placement_auto off); NULL is no pin, and Auto ignores it but keeps it.
ALTER TABLE agents ADD COLUMN device TEXT;
