-- Schema version 1 of the new base (docs/design/people-workspaces.md). A row's key is its kind letter and its id (p1, w2, a17):
-- AUTOINCREMENT, so an id is never reused. A column that names a who holds a key, as TEXT.

-- people: p<id>. handle is NULL in personal mode; role is owner or member.
CREATE TABLE people (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  display_name TEXT NOT NULL,
  handle TEXT UNIQUE,
  role TEXT NOT NULL CHECK (role IN ('owner', 'member')),
  created_at INTEGER NOT NULL
);
-- workspaces: w<id>. added_folders is a JSON list of paths; lead is the agent that receives what is sent to the workspace.
CREATE TABLE workspaces (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  folder TEXT NOT NULL,
  added_folders TEXT NOT NULL DEFAULT '[]',
  lead INTEGER REFERENCES agents(id),
  created_at INTEGER NOT NULL
);
-- agents: a<id>. owner is a person key, reports_to an agent or person key. A NULL setting means the default. schedules is a JSON
-- list of {cron, message}; role is work or check (a check agent's tasks gate other work).
CREATE TABLE agents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace INTEGER NOT NULL REFERENCES workspaces(id),
  name TEXT NOT NULL,
  owner TEXT NOT NULL,
  reports_to TEXT,
  role TEXT NOT NULL DEFAULT 'work' CHECK (role IN ('work', 'check')),
  enabled INTEGER NOT NULL DEFAULT 1,
  vendor TEXT,
  model TEXT,
  effort TEXT,
  weight TEXT,
  mode TEXT,
  context_tokens INTEGER,
  extra_worktrees INTEGER,
  handoff_note INTEGER,
  park_timeout_s INTEGER,
  sandbox INTEGER,
  sandbox_domains TEXT,
  chrome INTEGER,
  mcp_config TEXT,
  schedules TEXT,
  created_at INTEGER NOT NULL,
  UNIQUE (workspace, name)
);
CREATE INDEX agents_owner ON agents(owner);

CREATE TABLE events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  agent TEXT NOT NULL,
  account TEXT,
  kind TEXT NOT NULL,
  payload TEXT,
  dedup_key TEXT UNIQUE
);
CREATE TABLE requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  requester TEXT NOT NULL,
  target TEXT NOT NULL,
  body TEXT NOT NULL,
  files TEXT,
  relation TEXT,
  response TEXT,
  kind TEXT,
  chain_depth INTEGER NOT NULL DEFAULT 0,
  parent_id INTEGER,
  chain_id TEXT,
  state TEXT NOT NULL DEFAULT 'queued',
  queue_order REAL,
  delivered_at INTEGER,
  typed_at INTEGER,
  typed_session TEXT,
  landed_at INTEGER,
  send_now INTEGER,
  notice INTEGER,
  read_at INTEGER,
  state_at INTEGER,
  moot_by INTEGER,
  edge_ok INTEGER,
  ref TEXT,
  settles INTEGER,
  founder_auth INTEGER NOT NULL DEFAULT 0,
  no_work TEXT,
  no_work_by TEXT,
  memo_id INTEGER,
  quote TEXT
);
CREATE TABLE tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  owner TEXT NOT NULL,
  parent_task INTEGER REFERENCES tasks(id),
  opened_by TEXT NOT NULL,
  done_evidence TEXT,
  done_at INTEGER,
  done_via TEXT,
  edited_by TEXT,
  edited_at INTEGER,
  created_at INTEGER NOT NULL,
  closed_by TEXT,
  closed_at INTEGER,
  discarded_by TEXT,
  discard_reason TEXT,
  discarded_at INTEGER,
  priority INTEGER NOT NULL DEFAULT 2,
  merged_into INTEGER REFERENCES tasks(id),
  queued_by TEXT,
  queued_at INTEGER,
  timeout_s INTEGER,
  timeout_at INTEGER,
  delivered_at INTEGER,
  sent_at INTEGER,
  reporter TEXT,
  reported_at INTEGER,
  report TEXT,
  report_via TEXT,
  timeout_left INTEGER,
  expects TEXT,
  pr INTEGER,
  accepted_by TEXT,
  accepted_at INTEGER,
  estimate INTEGER,
  watch_vendor TEXT,
  watch_target TEXT,
  watch_until TEXT,
  watch_label TEXT,
  watch_result TEXT,
  watch_error TEXT,
  from_request INTEGER,
  request INTEGER,
  turn_origin INTEGER
);
CREATE TABLE task_requests (
  task_id INTEGER NOT NULL REFERENCES tasks(id),
  request_id INTEGER NOT NULL,
  PRIMARY KEY (task_id, request_id)
);
CREATE TABLE task_blocks (
  task_id INTEGER NOT NULL REFERENCES tasks(id),
  blocks_on INTEGER NOT NULL REFERENCES tasks(id),
  created_at INTEGER NOT NULL,
  dormant INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (task_id, blocks_on)
);
CREATE TABLE task_memos (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id),
  author TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  edited_at INTEGER,
  reply_to INTEGER
);
CREATE TABLE meta (
  name    TEXT PRIMARY KEY,
  value   TEXT,
  updated INTEGER NOT NULL
);
CREATE TABLE cursors (
  name    TEXT PRIMARY KEY,
  row_id  INTEGER NOT NULL,
  updated INTEGER NOT NULL
);
CREATE TABLE agent_state (
  agent TEXT PRIMARY KEY,
  state TEXT NOT NULL,
  wake_at INTEGER,
  detail TEXT,
  updated INTEGER NOT NULL,
  failed_attempts INTEGER NOT NULL DEFAULT 0,
  continuations INTEGER NOT NULL DEFAULT 0,
  throttle_count INTEGER NOT NULL DEFAULT 0,
  wake_failures INTEGER NOT NULL DEFAULT 0,
  terminal TEXT,
  worktree TEXT,
  account TEXT,
  closed_at INTEGER,
  resume_session TEXT,
  task TEXT,
  handoff TEXT,
  account_source TEXT,
  launch_parts TEXT,
  launch_fingerprint TEXT,
  extra_worktrees TEXT,
  notified_session TEXT,
  current_task INTEGER REFERENCES tasks(id),
  stall_kind TEXT,
  park_reason TEXT,
  stall_args TEXT
);
CREATE TABLE handoff (
  id TEXT PRIMARY KEY,
  agent TEXT NOT NULL,
  session_id TEXT,
  trigger_kind TEXT,
  step TEXT NOT NULL,
  deadline INTEGER,
  state TEXT NOT NULL,
  detail TEXT,
  updated INTEGER NOT NULL
);
CREATE TABLE delivery_blocks (
  agent TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  detail TEXT NOT NULL,
  work TEXT NOT NULL,
  since INTEGER NOT NULL,
  show_at INTEGER NOT NULL,
  seen INTEGER NOT NULL,
  request_id INTEGER,
  origin TEXT NOT NULL DEFAULT 'tick',
  title TEXT NOT NULL DEFAULT ''
);
CREATE TABLE dialog_presses (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent TEXT NOT NULL,
  at INTEGER NOT NULL,
  title TEXT NOT NULL,
  outcome TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT ''
);
CREATE TABLE prompt_blocks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent TEXT NOT NULL,
  kind TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  title TEXT NOT NULL,
  panel TEXT NOT NULL,
  detail TEXT NOT NULL,
  wait_id INTEGER,
  answerable INTEGER NOT NULL DEFAULT 1,
  first_seen INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  gone_at INTEGER,
  answered_at INTEGER,
  answer_ok INTEGER,
  answer_said TEXT,
  answer_label TEXT,
  answer_note TEXT,
  answer_by TEXT
);
CREATE TABLE usage_samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account TEXT NOT NULL,
  ts INTEGER NOT NULL,
  exists_ INTEGER NOT NULL,
  error TEXT,
  pct_5h INTEGER,
  pct_5h_readable INTEGER NOT NULL DEFAULT 1,
  reset_5h INTEGER,
  rl_at INTEGER,
  rl_carried INTEGER NOT NULL DEFAULT 0,
  rl_lost INTEGER NOT NULL DEFAULT 0,
  pct_7d INTEGER,
  reset_7d INTEGER,
  rl7_at INTEGER,
  rl7_carried INTEGER NOT NULL DEFAULT 0,
  rl7_lost INTEGER NOT NULL DEFAULT 0,
  collected_at INTEGER,
  windows TEXT,
  logged_out INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE pending_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent TEXT NOT NULL,
  text TEXT NOT NULL,
  ts INTEGER NOT NULL
);
CREATE TABLE token_calls (
  account TEXT NOT NULL,
  session TEXT NOT NULL,
  req TEXT NOT NULL,
  ts INTEGER NOT NULL,
  model TEXT NOT NULL,
  cwd TEXT NOT NULL,
  side INTEGER NOT NULL DEFAULT 0,
  inp INTEGER NOT NULL,
  cc INTEGER NOT NULL,
  cr INTEGER NOT NULL,
  out INTEGER NOT NULL,
  thinking INTEGER NOT NULL,
  cc1h INTEGER NOT NULL DEFAULT 0,
  agent TEXT NOT NULL DEFAULT '',
  task_id INTEGER,
  PRIMARY KEY (account, session, req)
);
CREATE TABLE session_starts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent TEXT NOT NULL,
  ts INTEGER NOT NULL,
  session_id TEXT,
  cause TEXT NOT NULL,
  resumed INTEGER NOT NULL,
  text TEXT NOT NULL,
  prompt TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  changed TEXT NOT NULL DEFAULT ''
);
CREATE INDEX events_agent_id ON events(agent, id);
CREATE INDEX events_kind_task ON events(kind, CASE WHEN json_valid(payload) THEN json_extract(payload, '$.task_id') END);
CREATE INDEX events_kind_request ON events(kind, CASE WHEN json_valid(payload) THEN json_extract(payload, '$.request_id') END);
CREATE INDEX requests_target ON requests(target, state);
CREATE INDEX requests_requester ON requests(requester, ts);
CREATE INDEX requests_chain ON requests(chain_id, id);
CREATE INDEX requests_parent ON requests(parent_id);
CREATE INDEX task_requests_request ON task_requests(request_id);
CREATE INDEX task_blocks_on ON task_blocks(blocks_on);
CREATE INDEX task_memos_task ON task_memos(task_id, id);
CREATE INDEX handoff_agent ON handoff(agent, state);
CREATE INDEX dialog_presses_agent ON dialog_presses(agent, at);
CREATE INDEX prompt_blocks_agent ON prompt_blocks(agent, gone_at);
CREATE UNIQUE INDEX prompt_blocks_open ON prompt_blocks(agent) WHERE gone_at IS NULL;
CREATE INDEX usage_samples_account_ts ON usage_samples(account, ts);
CREATE INDEX usage_samples_ts ON usage_samples(ts);
CREATE INDEX pending_messages_agent ON pending_messages(agent, id);
CREATE INDEX token_calls_account_ts ON token_calls(account, ts);
CREATE INDEX token_calls_ts ON token_calls(ts);
CREATE INDEX session_starts_agent ON session_starts(agent, id);
CREATE INDEX requests_settles ON requests(settles);
CREATE TABLE rpc_replies (
  caller TEXT NOT NULL,
  request_id TEXT NOT NULL,
  method TEXT NOT NULL,
  params TEXT NOT NULL,
  reply TEXT NOT NULL,
  ts INTEGER NOT NULL,
  PRIMARY KEY (caller, request_id)
);
CREATE INDEX rpc_replies_ts ON rpc_replies(ts);
CREATE TABLE agent_view (agent TEXT PRIMARY KEY, view TEXT NOT NULL, updated INTEGER NOT NULL);
CREATE TABLE stream_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  topic TEXT NOT NULL,
  type TEXT NOT NULL,
  agent TEXT,
  actor TEXT,
  title TEXT,
  target TEXT,
  data TEXT NOT NULL
);
CREATE INDEX stream_log_topic ON stream_log(topic, id);
CREATE TABLE hook_applied (
  request_id TEXT PRIMARY KEY,
  ts INTEGER NOT NULL,
  agent TEXT,
  at INTEGER
);
CREATE INDEX hook_applied_agent ON hook_applied(agent, at);
CREATE TABLE write_effects (
  caller TEXT NOT NULL,
  request_id TEXT NOT NULL,
  method TEXT NOT NULL,
  result_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (caller, request_id)
);
CREATE INDEX write_effects_created ON write_effects(created_at);
CREATE TABLE input_holders (
  agent TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  since INTEGER NOT NULL,
  last_typed INTEGER NOT NULL
);
CREATE TABLE operations (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  state TEXT NOT NULL,
  step TEXT,
  steps_done INTEGER NOT NULL DEFAULT 0,
  steps_total INTEGER,
  message TEXT,
  result_json TEXT,
  agent TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE devices (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL UNIQUE,
  person TEXT NOT NULL,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created INTEGER NOT NULL,
  last_seen INTEGER,
  push_platform TEXT,
  push_token TEXT,
  push_sandbox INTEGER,
  address TEXT,
  kind TEXT,
  client TEXT
);
CREATE TABLE transcript_events (
  agent TEXT NOT NULL,
  seq INTEGER NOT NULL,
  session TEXT NOT NULL,
  turn TEXT,
  at INTEGER NOT NULL,
  kind TEXT NOT NULL,
  item INTEGER,
  body TEXT NOT NULL,
  PRIMARY KEY (agent, seq)
);
CREATE TABLE agent_terminals (
  handle TEXT PRIMARY KEY,
  agent TEXT,
  opened_at INTEGER NOT NULL,
  closed_at INTEGER
);
CREATE INDEX requests_touched_at ON requests(COALESCE(state_at, ts));
CREATE INDEX requests_answered_ts ON requests(ts) WHERE settles IS NOT NULL;
CREATE UNIQUE INDEX requests_ref ON requests(ref);
CREATE TRIGGER requests_ref AFTER INSERT ON requests WHEN NEW.ref IS NULL
        BEGIN
          UPDATE requests SET ref = lower(hex(randomblob(6))) WHERE id = NEW.id;
        END;
CREATE TABLE task_tags (
          task_id INTEGER NOT NULL REFERENCES tasks(id),
          tag     TEXT    NOT NULL,
          PRIMARY KEY (task_id, tag)
        ) WITHOUT ROWID;
CREATE TABLE task_views (
  name TEXT PRIMARY KEY,
  filter TEXT NOT NULL,
  updated_by TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX task_tags_tag ON task_tags(tag, task_id);
CREATE TABLE prefs (
  person TEXT PRIMARY KEY,
  nav TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX tasks_from_request ON tasks(from_request) WHERE from_request IS NOT NULL;
CREATE TABLE memo_tells (
  memo_id INTEGER NOT NULL,
  reader TEXT NOT NULL,
  request_id INTEGER NOT NULL,
  PRIMARY KEY (memo_id, reader)
);
CREATE INDEX events_account_ts ON events(account, ts);
-- table_changes counts every write to a table a reader watches, so a reader looks again only when its tables moved.
CREATE TABLE table_changes (name TEXT PRIMARY KEY, n INTEGER NOT NULL DEFAULT 0) WITHOUT ROWID;
INSERT INTO table_changes(name) VALUES('agent_state');
INSERT INTO table_changes(name) VALUES('agent_view');
INSERT INTO table_changes(name) VALUES('agents');
INSERT INTO table_changes(name) VALUES('cursors');
INSERT INTO table_changes(name) VALUES('events');
INSERT INTO table_changes(name) VALUES('meta');
INSERT INTO table_changes(name) VALUES('operations');
INSERT INTO table_changes(name) VALUES('people');
INSERT INTO table_changes(name) VALUES('prefs');
INSERT INTO table_changes(name) VALUES('prompt_blocks');
INSERT INTO table_changes(name) VALUES('requests');
INSERT INTO table_changes(name) VALUES('session_starts');
INSERT INTO table_changes(name) VALUES('task_blocks');
INSERT INTO table_changes(name) VALUES('task_memos');
INSERT INTO table_changes(name) VALUES('task_requests');
INSERT INTO table_changes(name) VALUES('task_tags');
INSERT INTO table_changes(name) VALUES('task_turns');
INSERT INTO table_changes(name) VALUES('tasks');
INSERT INTO table_changes(name) VALUES('workspaces');
CREATE TRIGGER tc_people_i AFTER INSERT ON people
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'people'; END;
CREATE TRIGGER tc_people_u AFTER UPDATE ON people
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'people'; END;
CREATE TRIGGER tc_people_d AFTER DELETE ON people
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'people'; END;
CREATE TRIGGER tc_workspaces_i AFTER INSERT ON workspaces
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'workspaces'; END;
CREATE TRIGGER tc_workspaces_u AFTER UPDATE ON workspaces
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'workspaces'; END;
CREATE TRIGGER tc_workspaces_d AFTER DELETE ON workspaces
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'workspaces'; END;
CREATE TRIGGER tc_agents_i AFTER INSERT ON agents
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agents'; END;
CREATE TRIGGER tc_agents_u AFTER UPDATE ON agents
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agents'; END;
CREATE TRIGGER tc_agents_d AFTER DELETE ON agents
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agents'; END;
CREATE TRIGGER tc_agent_state_i AFTER INSERT ON agent_state
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agent_state'; END;
CREATE TRIGGER tc_agent_state_u AFTER UPDATE ON agent_state
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agent_state'; END;
CREATE TRIGGER tc_agent_state_d AFTER DELETE ON agent_state
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agent_state'; END;
CREATE TRIGGER tc_agent_view_i AFTER INSERT ON agent_view
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agent_view'; END;
CREATE TRIGGER tc_agent_view_u AFTER UPDATE ON agent_view
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agent_view'; END;
CREATE TRIGGER tc_agent_view_d AFTER DELETE ON agent_view
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'agent_view'; END;
CREATE TRIGGER tc_cursors_i AFTER INSERT ON cursors
  WHEN NEW.name NOT LIKE 'stream:%'
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'cursors'; END;
CREATE TRIGGER tc_cursors_u AFTER UPDATE ON cursors
  WHEN NEW.name NOT LIKE 'stream:%'
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'cursors'; END;
CREATE TRIGGER tc_cursors_d AFTER DELETE ON cursors
  WHEN OLD.name NOT LIKE 'stream:%'
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'cursors'; END;
CREATE TRIGGER tc_events_i AFTER INSERT ON events
  WHEN NEW.kind NOT IN ('verb_chain', 'rpc_write', 'UserPromptSubmit', 'Stop', 'SubagentStop', 'notified', 'SessionStart', 'SessionEnd')
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'events'; END;
CREATE TRIGGER tc_events_u AFTER UPDATE ON events
  WHEN NEW.kind NOT IN ('verb_chain', 'rpc_write', 'UserPromptSubmit', 'Stop', 'SubagentStop', 'notified', 'SessionStart', 'SessionEnd')
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'events'; END;
CREATE TRIGGER tc_events_d AFTER DELETE ON events
  WHEN OLD.kind NOT IN ('verb_chain', 'rpc_write', 'UserPromptSubmit', 'Stop', 'SubagentStop', 'notified', 'SessionStart', 'SessionEnd')
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'events'; END;
CREATE TRIGGER tc_meta_i AFTER INSERT ON meta
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'meta'; END;
CREATE TRIGGER tc_meta_u AFTER UPDATE ON meta
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'meta'; END;
CREATE TRIGGER tc_meta_d AFTER DELETE ON meta
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'meta'; END;
CREATE TRIGGER tc_operations_i AFTER INSERT ON operations
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'operations'; END;
CREATE TRIGGER tc_operations_u AFTER UPDATE ON operations
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'operations'; END;
CREATE TRIGGER tc_operations_d AFTER DELETE ON operations
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'operations'; END;
CREATE TRIGGER tc_prefs_i AFTER INSERT ON prefs
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'prefs'; END;
CREATE TRIGGER tc_prefs_u AFTER UPDATE ON prefs
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'prefs'; END;
CREATE TRIGGER tc_prefs_d AFTER DELETE ON prefs
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'prefs'; END;
CREATE TRIGGER tc_prompt_blocks_i AFTER INSERT ON prompt_blocks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'prompt_blocks'; END;
CREATE TRIGGER tc_prompt_blocks_u AFTER UPDATE ON prompt_blocks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'prompt_blocks'; END;
CREATE TRIGGER tc_prompt_blocks_d AFTER DELETE ON prompt_blocks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'prompt_blocks'; END;
CREATE TRIGGER tc_requests_i AFTER INSERT ON requests
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'requests'; END;
CREATE TRIGGER tc_requests_u AFTER UPDATE ON requests
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'requests'; END;
CREATE TRIGGER tc_requests_d AFTER DELETE ON requests
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'requests'; END;
CREATE TRIGGER tc_session_starts_i AFTER INSERT ON session_starts
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'session_starts'; END;
CREATE TRIGGER tc_session_starts_u AFTER UPDATE ON session_starts
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'session_starts'; END;
CREATE TRIGGER tc_session_starts_d AFTER DELETE ON session_starts
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'session_starts'; END;
CREATE TRIGGER tc_task_blocks_i AFTER INSERT ON task_blocks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_blocks'; END;
CREATE TRIGGER tc_task_blocks_u AFTER UPDATE ON task_blocks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_blocks'; END;
CREATE TRIGGER tc_task_blocks_d AFTER DELETE ON task_blocks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_blocks'; END;
CREATE TRIGGER tc_task_memos_i AFTER INSERT ON task_memos
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_memos'; END;
CREATE TRIGGER tc_task_memos_u AFTER UPDATE ON task_memos
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_memos'; END;
CREATE TRIGGER tc_task_memos_d AFTER DELETE ON task_memos
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_memos'; END;
CREATE TRIGGER tc_task_requests_i AFTER INSERT ON task_requests
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_requests'; END;
CREATE TRIGGER tc_task_requests_u AFTER UPDATE ON task_requests
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_requests'; END;
CREATE TRIGGER tc_task_requests_d AFTER DELETE ON task_requests
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_requests'; END;
CREATE TRIGGER tc_task_tags_i AFTER INSERT ON task_tags
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_tags'; END;
CREATE TRIGGER tc_task_tags_u AFTER UPDATE ON task_tags
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_tags'; END;
CREATE TRIGGER tc_task_tags_d AFTER DELETE ON task_tags
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_tags'; END;
CREATE TRIGGER tc_tasks_i AFTER INSERT ON tasks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'tasks'; END;
CREATE TRIGGER tc_tasks_u AFTER UPDATE ON tasks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'tasks'; END;
CREATE TRIGGER tc_tasks_d AFTER DELETE ON tasks
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'tasks'; END;
CREATE INDEX token_calls_task ON token_calls(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX token_calls_agent_ts ON token_calls(agent, ts) WHERE agent <> '';
CREATE TABLE load_minutes (
  minute INTEGER NOT NULL,
  agent TEXT NOT NULL,
  cpu_avg REAL NOT NULL,
  cpu_max REAL NOT NULL,
  rss_avg INTEGER NOT NULL,
  rss_max INTEGER NOT NULL,
  procs_max INTEGER NOT NULL,
  PRIMARY KEY (minute, agent)
) WITHOUT ROWID;
CREATE INDEX tasks_request ON tasks(request) WHERE request IS NOT NULL;
CREATE INDEX tasks_turn_origin ON tasks(turn_origin) WHERE turn_origin IS NOT NULL;
CREATE TABLE task_turns (
  task_id   INTEGER NOT NULL REFERENCES tasks(id),
  pos       INTEGER NOT NULL,
  agent     TEXT NOT NULL,
  ask       TEXT NOT NULL DEFAULT '',
  pushed_by TEXT NOT NULL,
  since     INTEGER NOT NULL,
  on_pass   TEXT,
  evidence  TEXT, memo INTEGER,
  PRIMARY KEY (task_id, pos)
) WITHOUT ROWID;
CREATE TRIGGER tc_task_turns_i AFTER INSERT ON task_turns
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_turns'; END;
CREATE TRIGGER tc_task_turns_u AFTER UPDATE ON task_turns
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_turns'; END;
CREATE TRIGGER tc_task_turns_d AFTER DELETE ON task_turns
BEGIN UPDATE table_changes SET n = n + 1 WHERE name = 'task_turns'; END;
CREATE TABLE account_switch (
  agent TEXT PRIMARY KEY,
  cause TEXT NOT NULL,
  from_account TEXT NOT NULL,
  to_account TEXT NOT NULL DEFAULT '',
  phase TEXT NOT NULL,
  since INTEGER NOT NULL
);
