-- heavy_runs: each heavy command an agent (its key) ran in the last 7 days (session.HeavyShape), by its shape only, never its text;
-- ended is when it ended, seconds how long it ran, job a background shell's id (kept once), NULL for a call timed by its hooks.
CREATE TABLE heavy_runs (
  agent TEXT NOT NULL,
  ended INTEGER NOT NULL,
  seconds INTEGER NOT NULL,
  shape TEXT NOT NULL,
  job TEXT
);
CREATE INDEX heavy_runs_agent ON heavy_runs(agent, ended);
CREATE INDEX heavy_runs_ended ON heavy_runs(ended);
CREATE UNIQUE INDEX heavy_runs_job ON heavy_runs(agent, job) WHERE job IS NOT NULL;
