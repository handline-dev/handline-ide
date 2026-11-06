-- agent_state.park_reason (added in store.go, addParkReason): why the daemon parked or queued an agent, kept as a column so the readers stop
-- reading it out of the detail's first word. The rows waiting now are filled from their detail once.
UPDATE agent_state SET park_reason = CASE
  WHEN substr(detail, 1, 6) = 'moving' THEN 'moving'
  WHEN substr(detail, 1, 7) = 'by hand' THEN 'by_hand'
  WHEN substr(detail, 1, 6) = 'usage;' OR substr(detail, 1, 9) = 'usage_7d;' THEN 'wall'
END WHERE state IN ('QUEUED', 'PARKED', 'DEGRADED', 'THROTTLED') AND detail IS NOT NULL;
