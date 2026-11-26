-- accounts.plan_fee: the account's monthly fee in US dollars when it is not its plan's listed one; NULL is the plan's.
ALTER TABLE accounts ADD COLUMN plan_fee REAL;
