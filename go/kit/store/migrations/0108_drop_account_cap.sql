-- accounts.active_cap goes: no per-account cap limits how many agents run on an account.
ALTER TABLE accounts DROP COLUMN active_cap;
