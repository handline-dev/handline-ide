-- The cursors of two once-only startup sweeps that withdrew stale comment asks; the sweeps are gone and nothing reads them.
DELETE FROM cursors WHERE name IN ('memo_asks_of_ended_tasks_withdrawn', 'memo_asks_unaddressed_withdrawn');
