-- How many times a lease was extended past its max_hold (hl use --extend, go/scheduler/leases), so a restart keeps its deadline.
ALTER TABLE leases ADD COLUMN extended INTEGER NOT NULL DEFAULT 0;
