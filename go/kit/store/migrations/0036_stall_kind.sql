-- agent_state.stall_kind (added in store.go, addStallKind): which stall the daemon raised, kept as a column so the screens stop reading it out of the detail's words. The
-- rows that are stalled now are filled from their detail once, in the order the cards were told apart.
UPDATE agent_state SET stall_kind = CASE
  WHEN instr(detail, 'folder-trust prompt') > 0 THEN 'trust'
  WHEN instr(detail, 'start-up dialog') > 0 THEN 'dialog'
  WHEN substr(detail, 1, 25) = 'cannot get past a dialog:' THEN 'send_dialog'
  WHEN instr(detail, 'starts for waiting work failed') > 0 THEN 'start'
  WHEN instr(detail, 'wakes started no session') > 0 THEN 'wake'
END WHERE state = 'STALLED' AND detail IS NOT NULL;
