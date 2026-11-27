-- agents.compact_at_task_end and agents.compact_now_stops_turn: an agent's own switches over the global ones.
ALTER TABLE agents ADD COLUMN compact_at_task_end INTEGER;
ALTER TABLE agents ADD COLUMN compact_now_stops_turn INTEGER;
