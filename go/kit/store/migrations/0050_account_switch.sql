-- account_switch: one row per agent, the account switch it is in or last was in (data/db/accountswitch.go says when a row is over).
CREATE TABLE IF NOT EXISTS account_switch (
  agent TEXT PRIMARY KEY,
  cause TEXT NOT NULL,
  from_account TEXT NOT NULL,
  to_account TEXT NOT NULL DEFAULT '',
  phase TEXT NOT NULL,
  since INTEGER NOT NULL
);
