-- agents.block_screen_capture: this agent may not capture a window through hl screen, over a global setting that is off.
ALTER TABLE agents ADD COLUMN block_screen_capture INTEGER;
