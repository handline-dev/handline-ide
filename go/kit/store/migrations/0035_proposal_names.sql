-- hl hr became hl propose: the rows that carry the old names (the stream's topic and event types, the screen a notice opens, the
-- outcome events in a chat, the method names of stored replies) say proposal now, so what was pending or recent still reads.
UPDATE stream_log SET topic = 'proposals' WHERE topic = 'hr';
UPDATE stream_log SET type = 'proposal.' || substr(type, 4) WHERE type IN ('hr.raised', 'hr.settled');
UPDATE stream_log SET target = REPLACE(target, '"screen":"hr"', '"screen":"proposals"') WHERE topic = 'proposals';
UPDATE events SET kind = 'proposal_outcome', dedup_key = REPLACE(dedup_key, 'hr_outcome:', 'proposal_outcome:') WHERE kind = 'hr_outcome';
UPDATE rpc_replies SET method = 'proposal.' || substr(method, 4) WHERE method LIKE 'hr.%';
UPDATE write_effects SET method = 'proposal.' || substr(method, 4) WHERE method LIKE 'hr.%';
