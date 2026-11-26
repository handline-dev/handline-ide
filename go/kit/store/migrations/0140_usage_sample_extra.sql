-- usage_samples.extra: the account's extra usage as that reading named it (on, cap, spent, currency) as JSON; NULL when unknown.
ALTER TABLE usage_samples ADD COLUMN extra TEXT;
