-- shape_days: how many timed Bash calls of each shape (session.HeavyShape, as if each had run long) ended on each day (epoch days),
-- and how many of them ran session.HeavyAfter or longer: a shape most of whose runs are long is a heavy run from its start. Kept a week.
CREATE TABLE shape_days (
  shape TEXT NOT NULL,
  day INTEGER NOT NULL,
  runs INTEGER NOT NULL DEFAULT 0,
  long INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (shape, day)
) WITHOUT ROWID;
-- heavy_runs.timed_out: 1 when the call failed at the time limit it gave itself.
ALTER TABLE heavy_runs ADD COLUMN timed_out INTEGER NOT NULL DEFAULT 0;
