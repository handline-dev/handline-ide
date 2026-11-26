-- tasks.watch_closes: on a gate, the task it closes when its watch passes (task.wait then done); NULL for a gate that only gives the
-- tasks it blocks back to their holders.
ALTER TABLE tasks ADD COLUMN watch_closes INTEGER REFERENCES tasks(id);
