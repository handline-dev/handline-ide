-- launch_fingerprint goes: a person's changed launch flag now starts a fresh session from a handoff note, so nothing compares a launch's
-- fingerprint any more.
ALTER TABLE agent_state DROP COLUMN launch_fingerprint;
