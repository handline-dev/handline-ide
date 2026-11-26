-- agents.account_split: the agent's own account_split (auto or manual) over the global setting; NULL follows the global one.
ALTER TABLE agents ADD COLUMN account_split TEXT;
