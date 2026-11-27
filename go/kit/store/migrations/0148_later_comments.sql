-- A comment can be a step for a person later (task_memos.later, their key; later_answered_at once their comment or pass answered it): it
-- asks nothing until its task's done or handoff brings the open ones to them as one turn. A turn whose pass hands the task on names whom
-- (task_turns.pass_to).
ALTER TABLE task_memos ADD COLUMN later TEXT;
ALTER TABLE task_memos ADD COLUMN later_answered_at INTEGER;
ALTER TABLE task_turns ADD COLUMN pass_to TEXT;
