-- A task's kept result names the branch its commit was measured against (branch) and whether that branch had it (landed), in place
-- of on_main. The rows kept before do not say which default branch it was; they are named main, which on_main was named for.
UPDATE meta SET value = json_remove(json_set(value, '$.branch', 'main', '$.landed', json(CASE WHEN json_extract(value, '$.on_main') THEN 'true' ELSE 'false' END)), '$.on_main')
WHERE name GLOB 'task-result:[0-9]*' AND json_valid(value) AND json_extract(value, '$.commit') IS NOT NULL;
