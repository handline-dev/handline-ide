-- session_starts.changed_lines: which lines of the text parts (team rules, team instructions, ROLE.md, base.md) a start changed, as JSON.
ALTER TABLE session_starts ADD COLUMN changed_lines TEXT NOT NULL DEFAULT '';
