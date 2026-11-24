-- agents.hired_from: for a temp (go/internal/temps), the key of the agent it was hired from; NULL for an agent of its own.
ALTER TABLE agents ADD COLUMN hired_from TEXT;
-- temps_left: a temp that left, so what still names it (its key, its name in its workspace) reaches the agent it was hired from.
CREATE TABLE temps_left (
  agent TEXT PRIMARY KEY,
  workspace INTEGER NOT NULL,
  name TEXT NOT NULL,
  hired_from TEXT NOT NULL,
  left_at INTEGER NOT NULL
);
