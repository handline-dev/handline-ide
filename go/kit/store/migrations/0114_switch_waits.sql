-- What a switch that has not begun its move waits for, in words the app shows: the turn to end, its handoff note, or background: <jobs>.
ALTER TABLE account_switch ADD COLUMN waits TEXT NOT NULL DEFAULT '';
