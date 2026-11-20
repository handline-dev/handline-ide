-- The kept rule lease_max.<kind> goes: a shared run is what takes turns (T9277).
DELETE FROM workspace_settings WHERE key LIKE 'lease\_max.%' ESCAPE '\';
