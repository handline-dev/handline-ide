-- nodes.door and nodes.door_fp: the address (host:port) the machine serves as master and its certificate's fingerprint ("" for a
-- plain door), the master's own from its listener and a runner's from its heartbeat, so a runner can find the master after a move.
ALTER TABLE nodes ADD COLUMN door TEXT;
ALTER TABLE nodes ADD COLUMN door_fp TEXT;
