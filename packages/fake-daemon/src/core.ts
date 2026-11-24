// The fake daemon's own methods: the handshake, who is calling, the subscription, the status and the agents, pairing, pull requests.
import { METHODS, PROTOCOL, ROLE_ORDER, type MethodName, type Role, type Timestamp, type Topic } from '@handline/contract';
import type { FakeAccounts } from '#src/accounts.ts';
import { MASTER_VERSION, type FakeAdmin } from '#src/admin.ts';
import { TERMINAL_CAPS, TEXT_CAPS, type Conn } from '#src/conn.ts';
import { badRequest, Failure, refused } from '#src/errors.ts';
import type { Script } from '#src/script.ts';
import type { EventLog } from '#src/eventlog.ts';
import type { ReviewLevel, ReviewWork } from '#src/tasks.ts';
import type { FakeInbox } from '#src/inbox.ts';
import type { FakePairing } from '#src/pairing.ts';
import { OWNER, PEOPLE } from '#src/names.ts';
import { subscribeTopics } from '#src/resume.ts';
import { detail, loadReply, newSession, statusReply, view, type Session, type TaskCounter } from '#src/session.ts';

/** What these methods need of the daemon that hosts them. */
export interface CoreHost {
  readonly sessions: Map<string, Session>;
  readonly tasks: TaskCounter & { reviewSummary(agent: string): { level: ReviewLevel; work: ReviewWork | null } };
  readonly inbox: FakeInbox;
  readonly admin: FakeAdmin;
  readonly script: Script;
  readonly accounts: FakeAccounts;
  readonly spaces: { folderFor(key: string): string | null };
  readonly replyItems: boolean;
  readonly pairing: FakePairing;
  readonly log: EventLog;
  readonly unservedTopics: Set<Topic>;
  readonly serveChat: boolean;
  readonly serveBytes: boolean;
  /** hello's capabilities.master: this daemon is master, with this node id and number; null for none named. */
  readonly master: { node: string; number: number } | null;
  readonly deviceRole: Role;
  readonly now: () => Date;
  iso(): Timestamp;
  /** Everything that may leave a method out of what this daemon lists and serves. */
  readonly served: { lists(name: string): boolean }[];
}

export const mayCall = (role: Role, lowest: Role): boolean => ROLE_ORDER.indexOf(role) >= ROLE_ORDER.indexOf(lowest);

export function hello(d: CoreHost, c: Conn, p: Record<string, unknown>): unknown {
  if (typeof p.client !== 'string') throw badRequest('hello needs client', 'client');
  if (typeof p.protocol !== 'number') throw badRequest('hello needs protocol', 'protocol');
  const device = c.network ? d.pairing.device(p.token) : undefined;
  if (device) {
    c.role = d.deviceRole;
    c.paired = true;
    c.caller = { kind: 'device', key: OWNER, device: device.id, name: device.name };
  }
  c.helloed = true;
  if (c.caller.name === 'unknown') c.caller = { ...c.caller, name: p.client };
  return {
    protocol: PROTOCOL,
    min_client: 1,
    daemon: 'fake0000',
    schema: 1,
    role: c.role,
    caller: c.caller,
    capabilities: {
      chat: d.serveChat,
      transcript: d.replyItems,
      review: false,
      ...(d.serveBytes ? TERMINAL_CAPS : TEXT_CAPS),
      // Its cluster's name, as the Go daemon's hello gives it (a runner, which the fake never is, gives none).
      ...(d.master ? { master: d.master } : {}),
      cluster: d.admin.cluster(d.iso()),
    },
    transport: c.network ? 'network' : 'local',
    methods: Object.entries(METHODS)
      .filter(([name]) => d.served.every((m) => m.lists(name)))
      .map(([name, m]) => ({ name, since: m.since, role: m.role, allowed: mayCall(c.role, m.role) })),
  };
}

/** The methods that belong to no area; refused as unsupported when it is none of them. */
export function coreCall(
  d: CoreHost,
  c: Conn,
  method: string,
  p: Record<string, unknown>,
  ok: (said: string, extra?: object) => object,
  agent: () => Session,
): unknown {
  switch (method as MethodName) {
    case 'whoami':
      return {
        caller: c.caller,
        role: c.role,
        methods: Object.entries(METHODS)
          .filter(([, m]) => mayCall(c.role, m.role))
          .map(([n]) => n),
      };
    case 'daemon.status':
      return statusReply(
        d.sessions,
        d.inbox.unread(),
        d.iso(),
        d.admin.machineNeeds,
        d.admin.keepAwake(),
        d.admin.paused,
        withInstall(d, missingTools(d)),
        withInstall(d, d.admin.vendors),
      );
    case 'load.get':
      return loadReply(d.sessions, d.iso(), p);
    case 'agents.list':
      return {
        agents: [...d.sessions.values()]
          .map((s) => view(s, d.tasks))
          .map((v) => ({
            ...v,
            review_level: d.tasks.reviewSummary(v.key).level,
            review_work: d.tasks.reviewSummary(v.key).work,
            folder: d.spaces.folderFor(v.key),
            node: d.admin.placed.get(v.key) ?? 'm-fake',
          })),
      };
    case 'people.list':
      return { people: PEOPLE };
    case 'node.status': {
      const at = d.iso();
      const own = { id: 'm-fake', person: OWNER, name: 'mac', joined_at: at, last_seen: at, master_number: 1 };
      const mine = {
        ...own,
        master: true,
        self: true,
        online: true,
        lending: 'run',
        device: null,
        memory_max_gb: null,
        load: null,
        battery: null,
        version: MASTER_VERSION,
      };
      const nodes: Record<string, unknown>[] = [mine, ...d.admin.lentNodes(at)];
      const nameOf = (id: string) => String(nodes.find((n) => n.id === id)?.name ?? id);
      const lackOn = (node: Record<string, unknown>, key: string) =>
        ((node.lacks ?? []) as { agent: string; why: string }[]).find((l) => l.agent === key)?.why;
      const HEAVY = 'heavy: checks 3 h this week';
      const manual = d.admin.settings().values.agent_placement === 'manual';
      // As the daemon places them: a pin on manual when its device can take the agent; a heavy agent on the lent machine it was seeded
      // on; one a lent machine lacks something for (the seed's "needs pencil", taken as heavy) here, saying why.
      const agents = [...d.sessions.values()].map((s) => {
        const { key } = view(s, d.tasks);
        const pin = s.settings.device ?? '';
        const placed = d.admin.placed.get(key);
        const kept = nodes.find((n) => !n.self && lackOn(n, key)?.startsWith('needs'));
        const pinned = manual && pin ? nodes.find((n) => n.id === pin) : undefined;
        const pinLack = pinned && lackOn(pinned, key);
        const [device, why] = pinned
          ? pinned.self
            ? [own.id, '']
            : pinLack
              ? [own.id, `pinned to ${nameOf(pin)}, which cannot take it now (${pinLack}), so it starts here`]
              : [pin, `pinned to ${nameOf(pin)}`]
          : placed
            ? [placed, HEAVY]
            : kept
              ? [own.id, `${HEAVY}; ${String(kept.name)} ${lackOn(kept, key)}, so it stays here`]
              : [own.id, ''];
        return { agent: key, device, why, heavy_s: placed || kept ? 3 * 3600 : 0, memory_peak: 2 ** 30, ...(pin ? { pin } : {}) };
      });
      return {
        master: { id: own.id, number: 1 },
        nodes,
        cluster: d.admin.cluster(at),
        placement: { mode: manual ? 'manual' : 'auto', agents },
      };
    }
    case 'agent.get':
      return detail(agent(), d.tasks);
    case 'subscribe': {
      c.topics = new Set(subscribeTopics(p, d.unservedTopics));
      return { cursor: d.log.head };
    }
    case 'unsubscribe':
      for (const t of (p.topics as Topic[] | undefined) ?? []) c.topics?.delete(t);
      return {};
    case 'prompt.list': {
      const s = agent();
      return { prompts: s.prompt ? [s.prompt] : [] };
    }
    case 'tools.install':
      return installTool(d, p);
    case 'device.pair':
      return d.pairing.pair(ok, d.now());
    case 'device.redeem':
      return d.pairing.redeem(ok, p);
    default:
      throw refused('unsupported', `the fake daemon does not serve ${method}`);
  }
}

/** The install window's session: no agent's, as a shell is the person's. */
const INSTALL_FX = {
  name: '(install)',
  role: '',
  model: '',
  effort: '',
  state: 'idle',
  account: null,
  task: null,
  worktree: '~',
  branch: '',
  session: null,
};

/** tools.install: a window that prints the tool's fix and runs until `endInstall` ends it; a running one is answered again. */
export function installTool(d: CoreHost, p: Record<string, unknown>): unknown {
  const tool = p.tool;
  if (tool !== 'claude' && tool !== 'codex' && tool !== 'git') throw badRequest('tool is claude, codex or git', 'tool');
  const was = installOf(d, tool);
  if (was && !was.ended) return { tool, terminal: was.terminal, host: 'fake' };
  const vendor = d.admin.vendors.find((v) => v.binary === tool && !v.installed);
  const fix = missingTools(d).find((m) => m.tool === tool)?.fix ?? vendor?.fix;
  if (!fix) throw new Failure('not_found', `${tool} is not missing`, { what: 'tool', name: tool });
  if (was) d.script.shells.delete(was.terminal); // an ended run's window gives way to the new one
  const terminal = `%sh${++d.script.shellCount}`;
  d.script.shells.set(terminal, {
    ...newSession(INSTALL_FX, d.iso()),
    handle: terminal,
    lines: [`$ ${fix}`],
    hasTerminal: true,
    open: false,
  });
  d.script.installs.set(tool, { terminal, ended: false });
  return { tool, terminal, host: 'fake' };
}

/** Ends a tool's running install: a failed one's window says so and stays; on success the window closes and the tool is installed. */
export function endInstall(d: CoreHost, tool: string, ok: boolean): void {
  const run = installOf(d, tool);
  if (!run || run.ended) throw new Error(`no install of ${tool} is running`);
  run.ended = true;
  if (!ok) {
    d.script.shells.get(run.terminal)!.lines.push('', 'Failed (exit 1).');
    return;
  }
  d.script.shells.delete(run.terminal);
  d.admin.missingTools = d.admin.missingTools.filter((m) => m.tool !== tool);
  const vendor = d.admin.vendors.find((v) => v.binary === tool);
  if (vendor) vendor.installed = true;
}

/** A tool's install window while its shell is open. */
function installOf(d: CoreHost, tool: string): { terminal: string; ended: boolean } | undefined {
  const run = d.script.installs.get(tool);
  return run && d.script.shells.has(run.terminal) ? run : undefined;
}

/** DaemonStatus' install on each missing tool and each vendor not installed, while its window is open. */
function withInstall<T extends { tool?: string; binary?: string; installed?: boolean }>(d: CoreHost, rows: readonly T[]): T[] {
  return rows.map((r) => {
    const run = r.installed ? undefined : installOf(d, (r.tool ?? r.binary)!);
    return run ? { ...r, install: { ...run } } : r;
  });
}

/** missing_tools: the seeded ones, and codex while it is not installed and an account uses it. */
function missingTools(d: CoreHost): { tool: string; fix: string }[] {
  const codex = d.admin.vendors.find((v) => v.vendor === 'codex' && !v.installed);
  const used = codex && d.accounts.list().accounts.some((a) => a.vendor === 'codex');
  return used ? [...d.admin.missingTools, { tool: 'codex', fix: codex.fix }] : d.admin.missingTools;
}
