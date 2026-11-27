-- Schema no code reads or writes goes: the dialog press log, the saved task views, the delivery blocks, three request columns, and the
-- task columns of the removed end-of-review step.
DROP TABLE dialog_presses;
DROP TABLE task_views;
DROP TABLE delivery_blocks;
DROP INDEX requests_settles;
DROP INDEX requests_answered_ts;
ALTER TABLE requests DROP COLUMN no_work;
ALTER TABLE requests DROP COLUMN no_work_by;
ALTER TABLE requests DROP COLUMN settles;
ALTER TABLE tasks DROP COLUMN reported_at;
ALTER TABLE tasks DROP COLUMN report;
ALTER TABLE tasks DROP COLUMN report_via;
ALTER TABLE tasks DROP COLUMN accepted_by;
ALTER TABLE tasks DROP COLUMN accepted_at;
