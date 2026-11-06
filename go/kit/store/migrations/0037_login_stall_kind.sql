-- A stall raised for an account that cannot be left has its own kind ('login'), so the lift reads the column and not the detail's words. The rows
-- stalled now are filled from the sentence the detail held until now.
UPDATE agent_state SET stall_kind = 'login'
WHERE state = 'STALLED' AND stall_kind IS NULL AND instr(detail, 'no other account can take it') > 0;
