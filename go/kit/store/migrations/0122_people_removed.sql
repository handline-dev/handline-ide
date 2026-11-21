-- people.removed_at: when the owner removed the person (person.remove). The row stays, so their past words, tasks and comments still name
-- them; their devices are revoked.
ALTER TABLE people ADD COLUMN removed_at INTEGER;
