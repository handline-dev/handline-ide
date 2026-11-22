// The fake daemon's own methods: the handshake, who is calling, the subscription, the status and the agents, pairing, pull requests.
import { METHODS, PROTOCOL, ROLE_ORDER, type MethodName, type Role, type Timestamp, type Topic } from '@handline/contract';
import type { FakeAccounts } from '#src/accounts.ts';
import { MASTER_VERSION, type FakeAdmin } from '#src/admin.ts';
import { TERMINAL_CAPS, TEXT_CAPS, type Conn } from '#src/conn.ts';
import { badRequest, refused } from '#src/errors.ts';
import type { EventLog } from '#src/eventlog.ts';
import type { ReviewLevel, ReviewWork } from '#src/tasks.ts';
import type { FakeInbox } from '#src/inbox.ts';
import type { FakePairing } from '#src/pairing.ts';
import { OWNER, PEOPLE } from '#src/names.ts';
import { subscribeTopics } from '#src/resume.ts';
import { detail, loadReply, statusReply, view, type Session, type TaskCounter } from '#src/session.ts';

/** What these methods need of the daemon that hosts them. */
export interface CoreHost {
  readonly sessions: Map<string, Session>;
  readonly tasks: TaskCounter & { reviewSummary(agent: string): { level: ReviewLevel; work: ReviewWork | null } };
  readonly inbox: FakeInbox;
  readonly admin: FakeAdmin;
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
      ...(d.master ? { master: d.master } : {}),
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
        missingTools(d),
        d.admin.vendors,
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
      return { master: { id: own.id, number: 1 }, nodes: [mine, ...d.admin.lentNodes(at)] };
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
    case 'device.pair':
      return d.pairing.pair(ok, d.now());
    case 'device.redeem':
      return d.pairing.redeem(ok, p);
    default:
      throw refused('unsupported', `the fake daemon does not serve ${method}`);
  }
}

/** missing_tools: the seeded ones, and codex while it is not installed and an account uses it. */
function missingTools(d: CoreHost): { tool: string; fix: string }[] {
  const codex = d.admin.vendors.find((v) => v.vendor === 'codex' && !v.installed);
  const used = codex && d.accounts.list().accounts.some((a) => a.vendor === 'codex');
  return used ? [...d.admin.missingTools, { tool: 'codex', fix: codex.fix }] : d.admin.missingTools;
}
