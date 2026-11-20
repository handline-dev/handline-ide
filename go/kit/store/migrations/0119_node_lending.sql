-- A lent machine's node row: the device it joined as (revoking the device drops the row) and its person's Run · Pause · Off.
-- run starts new work there, pause lets what runs finish, off is not lent. The master's own row has no device and is always run.
ALTER TABLE nodes ADD COLUMN device TEXT REFERENCES devices(id) ON DELETE CASCADE;
ALTER TABLE nodes ADD COLUMN lending TEXT NOT NULL DEFAULT 'off' CHECK (lending IN ('run', 'pause', 'off'));
UPDATE nodes SET lending = 'run' WHERE device IS NULL;
