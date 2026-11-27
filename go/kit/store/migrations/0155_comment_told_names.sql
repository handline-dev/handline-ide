-- The last stored names that said memo for a task comment: the comment a pull request comment was told in, and the cursor that
-- tells a comment's writer once that it went unread.
ALTER TABLE pr_comments_told RENAME COLUMN memo TO task_comment;
UPDATE cursors SET name = 'comment_lead:' || substr(name, 11) WHERE name GLOB 'memo_lead:*';
