// The fake daemon's read-mostly admin surface: the stats windows, the usage outlook, the home settings, the standing answers, and the repos
// and worktrees (worktrees.ts).
import { OWNER } from '#src/names.ts';
import type {
  ConnectionAddresses,
  Device,
  NetworkAddress,
  Outlook,
  MachineNeed,
  Settings,
  SettingsChanges,
  Timestamp,
} from '@handline/contract';
import { Failure, badRequest, refused } from '#src/errors.ts';
import { retroOf } from '#src/retro.ts';
import { AGENTS, outlookOf, statsOf } from '#src/usage.ts';
import { FakeWorktrees } from '#src/worktrees.ts';
import { FakePrefs, type PrefsHost } from '#src/prefs.ts';
import { BUILTIN_HANDOFF, DEFAULTS, EFFORTS } from '#src/settings-defaults.ts';

type Accepted = (said: string) => unknown;

/** The methods this answers. */
const ADMIN =
  /^(stats\.|connection\.addresses|settings\.|lease\.get|run\.(list|set|remove)|answer\.add|devices?\.(list|revoke|set|update)|events\.feed|daemon\.(pause|resume)|task\.send_now)/;

/** What a seeded lent machine's node row says beyond the defaults. */
export type Lent = Partial<{
  online: boolean;
  last_seen: Timestamp;
  load: { cores: number; load1: number; ram_total: number; ram_free: number; agents_rss: number };
  battery: boolean;
  lacks: { agent: string; why: string }[];
  /** Its Handline version, the master's (MASTER_VERSION) when left out. */
  version: string;
}>;

/** The fake master's Handline version, on its own node.status row. */
export const MASTER_VERSION = '5d6e7f8';

export class FakeAdmin {
  readonly worktrees: FakeWorktrees;
  readonly prefs: FakePrefs;
  private now: () => Timestamp;
  private on: boolean;
  private values: Settings['values'] = structuredClone(DEFAULTS);
  private devices: Device[] = [
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
      role: 'owner' as const,
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
  /** A device as the Device schema shows it, for an event. */
  device = (id: string): Record<string, unknown> => ({ ...this.devices.find((d) => d.id === id) });
  /** Each lent machine's own memory cap (device.set memory_max_gb), by device id. */
  private memoryMax = new Map<string, number>();
  /** A seeded lent machine's node: its id and what its row says beyond the defaults, by device id. */
  private lent = new Map<string, { node: string; row: Lent }>();
  /** A seed's cluster name. */
  rename = (name: string) => void (this.values.cluster_name = name);
  /** hello's and node.status's cluster: its name and machines, the master's own and those lent on Run or Pause. */
  cluster = (at: Timestamp) => ({ name: String(this.values.cluster_name), machines: 1 + this.lentNodes(at).length });
  /** node.status's rows of the machines lent on Run or Pause: a device with a node. */
  lentNodes = (at: Timestamp): Record<string, unknown>[] =>
    this.devices.flatMap((d) => {
      const n = this.lent.get(d.id);
      if (!n || (d.lending !== 'run' && d.lending !== 'pause')) return [];
      const version = n.row.version ?? MASTER_VERSION;
      // On another build it takes no agent (one lack shown here): Auto has it starting again on the master's, Manual leaves it to Update.
      const behind = `it runs Handline ${version}, not the master's ${MASTER_VERSION}`;
      const build =
        version === MASTER_VERSION
          ? []
          : [
              {
                agent: AGENTS[0]!,
                why:
                  this.values.node_build_auto === false
                    ? `${behind}: update to take work`
                    : `${behind}: it is starting again on the master's build`,
              },
            ];
      return [
        {
          id: n.node,
          person: OWNER,
          name: d.name,
          joined_at: at,
          last_seen: at,
          master_number: 1,
          master: false,
          self: false,
          online: true,
          lending: d.lending,
          device: d.id,
          memory_max_gb: this.memoryMax.get(d.id) ?? null,
          load: null,
          battery: null,
          ...n.row,
          version,
          lacks: [...build, ...(n.row.lacks ?? [])],
        },
      ];
    });
  /** Agents placed on a lent machine (the seed's), by agent key: its node id. Any other stays on the master's own. */
  readonly placed = new Map<string, string>();
  /** A seed's lent machine: a mac or server device on `lending`, and what its node row says beyond the defaults. */
  lend(d: {
    id: string;
    name: string;
    kind: 'mac' | 'server';
    lending: 'run' | 'pause' | 'off';
    connected?: boolean;
    lent?: Lent;
    agents?: string[];
  }) {
    const at = this.now();
    this.devices.push({
      id: d.id,
      name: d.name,
      role: 'owner' as const,
      created: at,
      last_seen: at,
      connected: d.connected ?? true,
      push: false,
      kind: d.kind,
      person: OWNER,
      lending: d.lending,
    });
    this.lent.set(d.id, { node: `n-${d.id}`, row: d.lent ?? {} });
    for (const a of d.agents ?? []) this.placed.set(a, `n-${d.id}`);
  }
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
      case 'device.set': {
        const dev = this.devices.find((d) => d.id === p.device);
        if (!dev) throw badRequest('no such device', 'device');
        if (p.lending !== undefined) Object.assign(dev, { lending: p.lending });
        if (p.memory_max_gb === null) this.memoryMax.delete(dev.id);
        else if (p.memory_max_gb !== undefined) {
          const gb = p.memory_max_gb;
          if (typeof gb !== 'number' || !Number.isInteger(gb) || gb < 1 || gb > 1048576)
            throw badRequest('memory_max_gb is a whole number of GB from 1 to 1048576, or null', 'memory_max_gb');
          this.memoryMax.set(dev.id, gb);
        }
        return accepted('Set.');
      }
      case 'device.update': {
        const n = this.lent.get(String(p.device));
        if (!n) throw badRequest('no such device', 'device');
        const dev = this.devices.find((d) => d.id === p.device);
        if (dev?.person !== OWNER) throw refused('person', 'only its own person updates it');
        if (n.row.online === false) throw refused('build', 'it is offline');
        if ((n.row.version ?? MASTER_VERSION) === MASTER_VERSION) throw refused('build', "it already runs the master's build");
        // The fake has it restart on the master's build at once.
        n.row = { ...n.row, version: MASTER_VERSION };
        return accepted('Updating.');
      }
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
    // Auto turned on: the owner's machines on another build restart on the master's at once.
    if (changes.node_build_auto === true)
      for (const [id, n] of this.lent)
        if (this.devices.find((d) => d.id === id)?.person === OWNER) n.row = { ...n.row, version: MASTER_VERSION };
    for (const [key, given] of Object.entries(changes)) {
      let value = given;
      if (key === 'handoff_prompt' && typeof value === 'string') {
        value = value.replace(/\s*[\r\n]+\s*/g, ' ').trim();
        if (value && value.length < 20) throw badRequest('Too short to guide a note. Leave it blank for the built-in.', 'handoff_prompt');
        if (!value) value = null;
      }
      if (key === 'cluster_name') {
        // as the daemon: trimmed, 1 to 40 characters (not UTF-16 units), no control or line-break character
        value = typeof value === 'string' ? value.trim() : value;
        if (
          typeof value !== 'string' ||
          !value ||
          [...value].length > 40 ||
          [...value].some((ch) => ch < ' ' || ch === '\u007f' || ch === '\u2028' || ch === '\u2029')
        )
          throw badRequest('cluster_name is a name of 1 to 40 characters on one line', 'cluster_name');
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
