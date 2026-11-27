-- token_calls.node: the machine that spent the call, as a node id; '' for a call kept from before, which counts as the master's.
ALTER TABLE token_calls ADD COLUMN node TEXT NOT NULL DEFAULT '';
