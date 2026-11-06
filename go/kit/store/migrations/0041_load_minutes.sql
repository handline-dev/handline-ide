-- load_minutes: what each agent's processes used, one row per agent per minute (internal/procload), kept for 7 days. agent '' is every
-- process no agent owns. Minute is epoch seconds rounded down to the minute; CPU is in percent of one core, memory is resident bytes.
CREATE TABLE IF NOT EXISTS load_minutes (
  minute INTEGER NOT NULL,
  agent TEXT NOT NULL,
  cpu_avg REAL NOT NULL,
  cpu_max REAL NOT NULL,
  rss_avg INTEGER NOT NULL,
  rss_max INTEGER NOT NULL,
  procs_max INTEGER NOT NULL,
  PRIMARY KEY (minute, agent)
) WITHOUT ROWID;
