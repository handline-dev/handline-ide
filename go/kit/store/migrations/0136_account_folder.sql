-- accounts.folder: the account's folder under accounts/; '' is a folder named by the row's key, as every account made before it.
ALTER TABLE accounts ADD COLUMN folder TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX accounts_folder ON accounts(folder) WHERE folder != '';
