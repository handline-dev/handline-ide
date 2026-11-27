-- The per-window columns of usage_samples go: a sample keeps its windows in the windows column, and no code reads or writes these.
ALTER TABLE usage_samples DROP COLUMN pct_5h;
ALTER TABLE usage_samples DROP COLUMN pct_5h_readable;
ALTER TABLE usage_samples DROP COLUMN reset_5h;
ALTER TABLE usage_samples DROP COLUMN rl_at;
ALTER TABLE usage_samples DROP COLUMN rl_carried;
ALTER TABLE usage_samples DROP COLUMN rl_lost;
ALTER TABLE usage_samples DROP COLUMN pct_7d;
ALTER TABLE usage_samples DROP COLUMN reset_7d;
ALTER TABLE usage_samples DROP COLUMN rl7_at;
ALTER TABLE usage_samples DROP COLUMN rl7_carried;
ALTER TABLE usage_samples DROP COLUMN rl7_lost;
