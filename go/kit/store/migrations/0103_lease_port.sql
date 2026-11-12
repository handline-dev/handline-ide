-- A port lease's number (go/scheduler/leases), so a restart keeps which port a lease holds.
ALTER TABLE leases ADD COLUMN port INTEGER NOT NULL DEFAULT 0;
