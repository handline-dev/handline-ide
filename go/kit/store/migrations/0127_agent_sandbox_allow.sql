-- agents.sandbox_allow: what an agent's sandboxed sessions may do beyond the sandbox, the founder's, as allowance words separated by
-- spaces (weaker-network, pnpm-install); NULL allows nothing more. It replaces the per-repo allowances of the protected store.
ALTER TABLE agents ADD COLUMN sandbox_allow TEXT;
