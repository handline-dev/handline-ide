-- leases: the simulator and browser leases held now (go/scheduler/leases), so a restart keeps them. workspace is the holder's
-- workspace key when it was granted; pid is the hl use process a command runs under, 0 for a hold.
CREATE TABLE leases (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  agent TEXT NOT NULL,
  workspace TEXT NOT NULL DEFAULT '',
  granted_at INTEGER NOT NULL,
  pid INTEGER NOT NULL DEFAULT 0
);
