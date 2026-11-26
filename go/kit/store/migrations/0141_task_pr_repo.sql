-- tasks.pr_repo: the repository of the task's pull request as its host names it (owner/name), beside the number in tasks.pr; NULL when
-- only a number was given.
ALTER TABLE tasks ADD COLUMN pr_repo TEXT;
