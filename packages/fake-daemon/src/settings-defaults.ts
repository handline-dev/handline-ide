// The home settings' defaults, as the Go daemon has them.
import type { Settings } from '@handline/contract';

/** The built-in handoff prompt (go/taskboard/handoff/note.go AskTemplate). */
export const BUILTIN_HANDOFF =
  "Write your handoff to {path} now, while your prompt cache is warm, with one Write (replace what is there), then stop and run nothing else. The next session of you, if it starts fresh, reads it first: copy yourself into it. At most {max} characters, about 30 lines, only what `hl` and your memory cannot tell it: (1) what you were deciding and why; (2) what you meant to do next; (3) promises or doubts not yet on any task; (4) the founder's preferences and the pitfalls you learned this session; (5) who you wait on, as T<n>/#<n> numbers only; (6) what slowed you down: one line each, starting with 'Slowed:', naming the command, tool, rule or T number and what it cost (retries, a wait, a refusal), or 'Slowed: none' if nothing did. Do not restate what a task or message holds; cite its T or # number. A lasting lesson or preference goes to your memory first, not here. If there is nothing, write the single word: nothing.";
/** Every settings value at the Go daemon's default (go/internal/reads/settings.go). */
export const DEFAULTS: Settings['values'] = {
  park_timeout_s: 1800,
  handoff_note: true,
  ignore_cache_expiry: false,
  dev_mode: false,
  bg_notice_s: 1800,
  bg_grace_s: 300,
  bg_check_s: 600,
  notice_digest_s: 300,
  token_prices: {},
  input_hold_idle_s: 600,
  handoff_prompt: BUILTIN_HANDOFF,
  push_prompt_asked: true,
  push_task_done: true,
  push_stall: true,
  push_account_out: true,
  push_only_away: false,
  push_sound: true,
  account_split: 'auto',
  block_screen_capture: false,
  screencapture_via_handline: false,
  agent_placement: 'auto',
  review_flow: false,
  landed_deadline_s: 604800,
  pace_target_pct: 98,
  pace_burst_pct: 10,
  host_active_cap: 0,
  load_per_core_pct: 0,
  ram_free_min_pct: 0,
  disk_free_min_gb: 0,
  disk_keep_free_gb: 0,
  agents_memory_max_gb: 0,
  node_battery_switch: false,
  node_reclaim_s: 1800,
  cluster_name: 'Blue Harbor',
  node_build_auto: true,
  port_range: null,
  network_addresses: null,
  keep_awake: 'off',
  reap_orphans: 'dry',
  default_model_claude: 'sonnet',
  default_effort_claude: 'medium',
  default_model_codex: '',
  default_effort_codex: 'medium',
};
/** The efforts a model takes. */
export const EFFORTS = ['low', 'medium', 'high', 'xhigh', 'max'];
