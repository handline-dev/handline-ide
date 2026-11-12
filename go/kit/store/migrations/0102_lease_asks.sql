-- A lease reserved for a task (task, until: the claim deadline) and an ask waiting in line (waiting 1, task, gate: the gate task that
-- blocks it), so a restart keeps the line (go/scheduler/leases).
ALTER TABLE leases ADD COLUMN task INTEGER NOT NULL DEFAULT 0;
ALTER TABLE leases ADD COLUMN gate INTEGER NOT NULL DEFAULT 0;
ALTER TABLE leases ADD COLUMN until INTEGER NOT NULL DEFAULT 0;
ALTER TABLE leases ADD COLUMN waiting INTEGER NOT NULL DEFAULT 0;
