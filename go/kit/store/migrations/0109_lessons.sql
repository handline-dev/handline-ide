-- lessons: what each agent learned (from a Redo), carried in its launch prompt. A row is never deleted: a removal or a merge marks it,
-- with who and when, and a merged row names the lesson it went into.
CREATE TABLE lessons (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent TEXT NOT NULL,
  text TEXT NOT NULL,
  task INTEGER,
  created_at INTEGER NOT NULL,
  created_by TEXT NOT NULL,
  removed_at INTEGER,
  removed_by TEXT,
  merged_into INTEGER
);
CREATE INDEX lessons_agent ON lessons(agent, removed_at);
