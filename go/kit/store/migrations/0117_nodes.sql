-- nodes: the machines of this master, its own first. id is the machine's node id (<home>/node_id); person is whose machine it is;
-- master_number is the master it belongs to, 1 until masters can change.
CREATE TABLE nodes (
  id TEXT PRIMARY KEY,
  person INTEGER REFERENCES people(id),
  name TEXT NOT NULL,
  joined_at INTEGER NOT NULL,
  last_seen INTEGER,
  master_number INTEGER NOT NULL DEFAULT 1
);
INSERT INTO table_changes(name) VALUES('nodes');
CREATE TRIGGER tc_nodes_i AFTER INSERT ON nodes
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'nodes'; END;
CREATE TRIGGER tc_nodes_u AFTER UPDATE ON nodes
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'nodes'; END;
CREATE TRIGGER tc_nodes_d AFTER DELETE ON nodes
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'nodes'; END;
