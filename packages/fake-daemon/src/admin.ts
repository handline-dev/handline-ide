// The fake daemon's read-mostly admin surface: the stats windows, the usage outlook, the home settings, the standing answers, and the repos
// and worktrees (worktrees.ts).
import type { ConnectionAddresses, NetworkAddress, Outlook, MachineNeed, Settings, SettingsChanges, Timestamp } from '@handline/contract';
import { Failure, badRequest } from '#src/errors.ts';
import { retroOf } from '#src/retro.ts';
import { AGENTS, outlookOf, statsOf } from '#src/usage.ts';
import { FakeWorktrees } from '#src/worktrees.ts';
import { FakePrefs, type PrefsHost } from '#src/prefs.ts';

type Accepted = (said: string) => unknown;

/** The methods this answers. */
const ADMIN =
  /^(stats\.|connection\.addresses|settings\.|lease\.get|run\.(list|set|remove)|answer\.add|devices?\.(list|revoke)|events\.feed|daemon\.(pause|resume)|task\.send_now)/;
/** The built-in handoff prompt (go/taskboard/handoff/note.go AskTemplate). */
const BUILTIN_HANDOFF =
  "Write your handoff to {path} now, while your prompt cache is warm, with one Write (replace what is there), then stop and run nothing else. The next session of you, if it starts fresh, reads it first: copy yourself into it. At most {max} characters, about 30 lines, only what `hl` and your memory cannot tell it: (1) what you were deciding and why; (2) what you meant to do next; (3) promises or doubts not yet on any task; (4) the founder's preferences and the pitfalls you learned this session; (5) who you wait on, as T<n>/#<n> numbers only; (6) what slowed you down: one line each, starting with 'Slowed:', naming the command, tool, rule or T number and what it cost (retries, a wait, a refusal), or 'Slowed: none' if nothing did. Do not restate what a task or message holds; cite its T or # number. A lasting lesson or preference goes to your memory first, not here. If there is nothing, write the single word: nothing.";
/** Every settings value at the Go daemon's default (go/internal/reads/settings.go). */
const DEFAULTS: Settings['values'] = {
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
  review_flow: false,
  pace_target_pct: 98,
  pace_burst_pct: 10,
  host_active_cap: 0,
  load_per_core_pct: 0,
  ram_free_min_pct: 0,
  disk_free_min_gb: 0,
  disk_keep_free_gb: 0,
  agents_memory_max_gb: 0,
  port_range: null,
  network_addresses: null,
  keep_awake: 'off',
  reap_orphans: 'dry',
  default_model_claude: 'sonnet',
  default_effort_claude: 'medium',
  default_model_codex: '',
  default_effort_codex: 'medium',
};
const EFFORTS = ['low', 'medium', 'high', 'xhigh', 'max'];

export class FakeAdmin {
  readonly worktrees: FakeWorktrees;
  readonly prefs: FakePrefs;
  private now: () => Timestamp;
  private on: boolean;
  private values: Settings['values'] = structuredClone(DEFAULTS);
  private devices = [
    {
      id: 'd1',
      name: 'Handline desktop',
      role: 'owner' as const,
      created: '2026-10-01T09:00:00Z',
      last_seen: '2026-10-16T01:00:00Z',
      connected: true,
      push: false,
    },
    {
      id: 'd2',
      name: 'phone-3b54',
      role: 'owner' as const,
      created: '2026-10-05T09:00:00Z',
      last_seen: '2026-10-12T08:00:00Z',
      connected: false,
      push: true,
    },
    ...Array.from({ length: 12 }, (_, i) => ({
      id: `d${i + 3}`,
      name: `phone-${(0x3c10 + i * 37).toString(16)}`,
      role: 'founder' as const,
      created: '2026-08-01T09:00:00Z',
      last_seen: `2026-09-${String(1 + i).padStart(2, '0')}T08:00:00Z`,
      connected: false,
      push: i % 2 === 0,
    })),
  ];

  /** The machine's own needs-you items in daemon.status (a full disk); `setMachineNeeds` of helpers.ts changes them. */
  machineNeeds: MachineNeed[] = [];

  /** DaemonStatus.missing_tools: the tools agents need that this machine lacks; the first-run seed sets them. */
  missingTools: { tool: string; fix: string }[] = [];
  /** DaemonStatus.vendors: what agents can run with here, installed or not; a test marks one missing. */
  vendors: { vendor: string; binary: string; installed: boolean; fix: string }[] = [
    { vendor: 'claude', binary: 'claude', installed: true, fix: 'npm install -g @anthropic-ai/claude-code' },
    { vendor: 'codex', binary: 'codex', installed: false, fix: 'npm install -g @openai/codex' },
  ];

  /** daemon.pause on this machine (DaemonStatus.paused), null while new work runs. */
  paused: { since: Timestamp; until: null; machine: { id: string; name: string } } | null = null;

  /** `on` false is a daemon that does not serve stats, settings or standing answers: it lists none of their methods. */
  constructor(now: () => Timestamp, on: boolean, host: PrefsHost) {
    this.now = now;
    this.on = on;
    this.worktrees = new FakeWorktrees(on);
    this.prefs = new FakePrefs(host);
  }

  /** Whether the daemon lists this method. */
  lists = (name: string): boolean => (this.on || !ADMIN.test(name)) && this.worktrees.lists(name);

  /** Whether this class answers the method. */
  owns = (name: string): boolean => ADMIN.test(name) || this.worktrees.owns(name) || this.prefs.owns(name);

  call(method: string, p: Record<string, unknown>, accepted: Accepted): unknown {
    if (this.worktrees.owns(method)) return this.worktrees.call(method, p);
    if (this.prefs.owns(method)) return this.prefs.call(method, p, accepted);
    switch (method) {
      case 'stats.get':
        return statsOf(this.now(), p.window === '7d' ? '7d' : '24h');
      case 'stats.outlook':
        return outlookOf(this.now(), String(p.range) as Outlook['range'], p.agent);
      case 'stats.retro':
        return retroOf(Number(p.task));
      case 'connection.addresses':
        return this.addresses();
      case 'settings.get':
        return this.settings();
      case 'settings.set':
        return (this.set((p.changes ?? {}) as SettingsChanges), accepted('Saved.'));
      case 'lease.get':
        return this.leases();
      case 'run.list':
        return { runs: this.runs, presets: ['@build-test', '@simulator', '@browser'] };
      case 'run.set': {
        const scope = p.scope === 'workspace' ? String(p.workspace) : 'mac';
        const old = this.runs.find((r) => r.name === p.name && r.scope === scope);
        const run = {
          kind: `${scope}:${String(p.name)}`,
          name: String(p.name),
          scope,
          cmds: (p.cmds as string[]) ?? [],
          at_once: Number(p.at_once ?? 1),
          why: String(p.why ?? ''),
          set_by: 'human',
          set_at: this.now(),
          held: old?.held ?? [],
          waiting: old?.waiting ?? [],
        };
        this.runs = [...this.runs.filter((r) => r !== old), run];
        return accepted('Kept.');
      }
      case 'run.remove': {
        const scope = p.scope === 'workspace' ? String(p.workspace) : 'mac';
        this.runs = this.runs.filter((r) => !(r.name === p.name && r.scope === scope));
        return accepted('Removed.');
      }
      case 'events.feed':
        return {
          events: [
            {
              cursor: 2,
              topic: 'tasks',
              type: 'task.changed',
              at: this.now(),
              actor: null,
              agent: AGENTS[1],
              title: 'Task T7 done',
              target: null,
            },
            {
              cursor: 1,
              topic: 'agents',
              type: 'agent.state',
              at: this.now(),
              actor: null,
              agent: AGENTS[0],
              title: 'Started',
              target: null,
            },
          ],
          cursor: 2,
          gap: false,
        };
      case 'devices.list':
        return { devices: this.devices.map((d) => ({ ...d })) };
      case 'device.revoke':
        if (!this.devices.some((d) => d.id === p.device)) throw badRequest('no such device', 'device');
        return ((this.devices = this.devices.filter((d) => d.id !== p.device)), accepted('Revoked.'));
      case 'answer.add':
        return (this.add(p), accepted('Added.'));
      case 'daemon.pause':
        this.paused = { since: this.now(), until: null, machine: { id: 'm-fake', name: 'mac' } };
        return accepted('Paused.');
      case 'daemon.resume':
        this.paused = null;
        return accepted('Resumed.');
      case 'task.send_now':
        if (!this.paused) throw new Failure('refused', 'nothing is paused', { reason: 'not_paused' });
        return accepted(`T${String(p.task)} goes to its agent now.`);
      default:
        throw new Failure('refused', `the fake daemon does not serve ${method}`, { reason: 'unsupported', gate: 'unsupported' });
    }
  }

  /** A LAN address, Tailscale's and their names; the LAN one listens unless network_addresses chose others. */
  addresses(): ConnectionAddresses {
    const chosen = this.values.network_addresses ?? null;
    const listening = (host: string) => (chosen ? chosen.includes(host) : host === '192.168.1.20');
    const row = (host: string, kind: NetworkAddress['kind'], iface: string): NetworkAddress => ({
      host,
      kind,
      interface: iface,
      listening: listening(host),
      encryption: kind === 'lan' || kind === 'mdns' ? 'tls' : 'none',
    });
    return {
      addresses: [
        row('192.168.1.20', 'lan', 'en0'),
        row('100.101.1.2', 'tailscale', 'utun3'),
        row('mac.tail1.ts.net', 'tailscale_name', 'utun3'),
        row('mac.local', 'mdns', ''),
      ],
      network_addresses: chosen,
    };
  }

  /** The keep_awake mode the settings say (DaemonStatus.keep_awake.mode). */
  keepAwake(): string {
    return this.values.keep_awake ?? 'off';
  }

  /** Shared runs (run.list): one of This Mac's held with one waiting, and one of a workspace's. */
  private runs = [
    {
      kind: 'mac:ios-build',
      name: 'ios-build',
      scope: 'mac',
      cmds: ['xcodebuild', '@simulator'],
      at_once: 1,
      why: 'One simulator build at a time keeps the fans down.',
      set_by: AGENTS[0],
      set_at: new Date(Date.now() - 3_600_000).toISOString(),
      held: [{ agent: AGENTS[1], since: new Date(Date.now() - 300_000).toISOString(), hold: false }],
      waiting: [{ agent: AGENTS[2], since: new Date(Date.now() - 120_000).toISOString(), position: 1 }],
    },
    {
      kind: 'w1:check',
      name: 'check',
      scope: 'w1',
      cmds: ['tools/check.sh'],
      at_once: 2,
      why: 'check.sh takes every core.',
      set_by: AGENTS[0],
      set_at: new Date(Date.now() - 7_200_000).toISOString(),
      held: [] as { agent: string; since: string; hold: boolean }[],
      waiting: [] as { agent: string; since: string; position: number; task?: number }[],
    },
  ];
  /** ios-build: the architect holds it, and the second agent waits, its ask blocking `task`. */
  waitForRun(task: number): void {
    this.runs[0]!.held = [{ agent: AGENTS[0], since: this.now(), hold: false }];
    this.runs[0]!.waiting = [{ agent: AGENTS[1], since: this.now(), position: 1, task }];
  }

  /** The shared runs as lease pools: who holds a turn and who waits. */
  leases() {
    return {
      kinds: this.runs.map((r) => ({
        kind: r.kind,
        max: r.at_once,
        held: r.held.map((h, i) => ({ lease: i + 1, agent: h.agent, since: h.since, hold: h.hold, pids: [] })),
        queue: r.waiting.map((w) => ({ ...w, says: `waiting for a turn at ${r.name}, number ${w.position} in line` })),
      })),
    };
  }

  settings(): Settings {
    const values = structuredClone(this.values) as Record<string, unknown>;
    return {
      timings: Object.fromEntries(Object.entries(values).filter(([k]) => k.endsWith('_s'))),
      terminal_host: { choice: 'fake', available: ['fake'], health: 'ok' },
      trust_folders: [],
      hooks: { installed: true, accounts: [] },
      values,
      handoff_prompt_default: BUILTIN_HANDOFF,
      handoff_prompt_own: [...this.handoffOwn],
    } as unknown as Settings;
  }

  /** Agents whose own agent.yaml sets a handoff prompt, which wins over this one. */
  handoffOwn: string[] = [];

  set(changes: SettingsChanges): void {
    // As the real daemon does: a key the contract does not list is refused, so an app writing a key nobody listed fails its test.
    for (const key of Object.keys(changes)) if (!(key in DEFAULTS)) throw badRequest(`${key} is not a setting`, key);
    for (const [key, given] of Object.entries(changes)) {
      let value = given;
      if (key === 'handoff_prompt' && typeof value === 'string') {
        value = value.replace(/\s*[\r\n]+\s*/g, ' ').trim();
        if (value && value.length < 20) throw badRequest('Too short to guide a note. Leave it blank for the built-in.', 'handoff_prompt');
        if (!value) value = null;
      }
      if (key === 'park_timeout_s' && value !== null && (!Number.isInteger(value) || (value as number) < 0))
        throw badRequest('park_timeout_s is a whole number of seconds, 0 for never', 'park_timeout_s');
      if (key === 'keep_awake' && value !== null && value !== 'off' && value !== 'on' && value !== 'working')
        throw badRequest('keep_awake is off, on or working', 'keep_awake');
      if (key === 'reap_orphans' && value !== null && value !== 'dry' && value !== 'on' && value !== 'off')
        throw badRequest('reap_orphans is dry, on or off', 'reap_orphans');
      if (key.startsWith('default_effort_') && value !== null && !EFFORTS.includes(value as string))
        throw badRequest(`${key} is one of ${EFFORTS.join(', ')}`, key);
      if (key.startsWith('default_model_') && value !== null && (typeof value !== 'string' || !/^(?!-)[^\s'"]{1,100}$/.test(value)))
        throw badRequest(`${key} is a model name of at most 100 characters, no spaces, not starting with -`, key);
      const values = this.values as Record<string, unknown>;
      if (value === null) {
        values[key] = structuredClone(DEFAULTS[key as keyof typeof DEFAULTS]);
      } else values[key] = value;
    }
  }

  /** answer.add: a folder, or a dialog rule's vendor, title and label; the fake keeps none. */
  private add(p: Record<string, unknown>): void {
    const given = (k: string) => typeof p[k] === 'string' && (p[k] as string).trim() !== '';
    if (!given('folder') && !['vendor', 'title', 'label'].every(given))
      throw badRequest('give a folder, or vendor, title and label', 'folder');
  }
}
