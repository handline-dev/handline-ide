-- review_levels: how much of an agent's story work the founder looks at (every, one_in_three, auto); the newest row is the level now,
-- none is every. Each change is a row with who, when and why (set, or a Redo with its task).
CREATE TABLE review_levels (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent TEXT NOT NULL,
  level TEXT NOT NULL,
  at INTEGER NOT NULL,
  by TEXT NOT NULL,
  why TEXT NOT NULL,
  task INTEGER,
  after_event INTEGER NOT NULL -- the newest event when it was set: the looks at this level are the verdicts after it
);
CREATE INDEX review_levels_agent ON review_levels(agent, id);
