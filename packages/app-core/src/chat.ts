// The pure side of chat: how messages and transcript items are ordered, who a message is from, and what can still be done to it.
// Fetching and keeping the conversation current is the apps' shared query layer (@handline/app-data).
import type { ChatMessage, Proposal, TranscriptItem } from '@handline/client';

export type TimelineEntry =
  | { kind: 'message'; at: string; id: number; message: ChatMessage }
  /** The task board's word about a task (`T<N> done by x: ...`), drawn as a task card, not a bubble. */
  | { kind: 'note'; at: string; id: number; message: ChatMessage; task: number }
  | { kind: 'item'; at: string; id: number; item: TranscriptItem }
  /** An proposal that was decided: it keeps its place at the time it was settled. */
  | { kind: 'proposal'; at: string; id: number; proposal: Proposal };

/** The sender of the daemon's own notices; its words are never the person's. */
export const NOTICE_SENDER = 'sys';
/** What a sys message is: a task's news (`T<N> done by ...`), or a delivery notice (`#<N> to x was written ...`) that only concerns the sender. */
export function noticeOf(m: ChatMessage): { kind: 'task'; task: number } | { kind: 'delivery' } | null {
  if (m.from !== NOTICE_SENDER) return null;
  const task = /^T(\d+)\s/.exec(m.body);
  if (task) return { kind: 'task', task: Number(task[1]) };
  return /^#\d+\s/.test(m.body) ? { kind: 'delivery' } : null;
}

/** A permission request is a tool step (what the agent asked to run), so it is Activity's like the tool line; other dialogs stay in Chat. */
const isToolStep = (i: TranscriptItem) => i.kind === 'tool' || (i.kind === 'prompt' && i.prompt?.kind === 'permission');

/** Everything on screen for one agent, oldest first: the messages and the agent's transcript by time. In the agent's own chat (`mine`) nothing the task board sent and no tool line is shown: they still reach the agent and stay on the Activity tab. */
export function timeline(
  messages: ChatMessage[],
  items: TranscriptItem[],
  scope: 'mine' | 'all' = 'mine',
  agent?: string,
  me = '',
  decided: readonly Proposal[] = [],
  /** Keep tool steps in the agent's own chat too, for foldTurns. */
  steps = false,
): TimelineEntry[] {
  const out: TimelineEntry[] = [
    ...messages.flatMap((m): TimelineEntry[] => {
      if (scope === 'mine' && m.from === NOTICE_SENDER) return [];
      if (scope === 'mine' && m.proposal != null) return []; // the outcome of a proposal is drawn as its card, not as this line
      // What this agent says to another agent is that agent's room's, not this one's: the all tab shows what reaches the agent and what it says to the person.
      if (scope === 'all' && agent && m.from === agent && m.to !== agent && !isPerson(m.to, me)) return [];
      const n = noticeOf(m);
      if (n?.kind === 'task') return [{ kind: 'note', at: m.at, id: m.id, message: m, task: n.task }];
      return [{ kind: 'message', at: m.at, id: m.id, message: m }];
    }),
    // A task card sits in the chat of the agent that made the change, or of the founder acting for it (`human`); the *all* tab shows every one.
    // An unblocked card is the exception: its `who` closed the blocker, and it belongs to the agent the task starts for.
    ...items
      .filter(
        (i) =>
          !(
            scope === 'mine' &&
            ((!steps && isToolStep(i)) ||
              (agent && i.task_event && i.task_event.event !== 'unblocked' && i.task_event.who !== agent && i.task_event.who !== 'human'))
          ),
      )
      .map((i): TimelineEntry => ({ kind: 'item', at: i.at, id: i.id, item: i })),
    ...decided.flatMap((p): TimelineEntry[] => (p.settled ? [{ kind: 'proposal', at: p.settled, id: p.id, proposal: p }] : [])),
  ];
  const rank = (e: TimelineEntry) => (e.kind === 'item' ? 1 : e.kind === 'proposal' ? 2 : 0); // at one instant: messages, then the transcript, then a decision
  const starts = items.filter((i) => i.kind === 'session').map((i) => i.at);
  // A message that waited through a restart and reached the new session reads below that session's line, not above it.
  // Typed is when it entered the terminal: a message that wakes an agent is marked delivered before its new session starts.
  const waited = new Map(
    out.flatMap((e): [TimelineEntry, string][] => {
      const sent = e.kind === 'message' ? (e.message.typed_at ?? e.message.delivered_at) : null;
      const start = sent ? starts.filter((at) => at > e.at && at <= sent).at(-1) : undefined;
      return start ? [[e, start]] : [];
    }),
  );
  const at = (e: TimelineEntry) => waited.get(e) ?? e.at;
  const order = (e: TimelineEntry) => (waited.has(e) ? 3 : rank(e));
  out.sort((a, b) => (at(a) < at(b) ? -1 : at(a) > at(b) ? 1 : order(a) - order(b) || a.id - b.id));
  return out.reduce<TimelineEntry[]>((all, e) => {
    const prev = all.at(-1);
    // A restart after a new conversation is its own event; a resume that failed into a fresh start is one.
    const restartAfterNew = prev?.kind === 'item' && e.kind === 'item' && !prev.item.session?.resumed && e.item.session?.resumed;
    if (prev?.kind === 'item' && e.kind === 'item' && prev.item.kind === 'session' && e.item.kind === 'session' && !restartAfterNew)
      all[all.length - 1] = mergeSessions(prev, e);
    else all.push(e);
    return all;
  }, []);
}

/** The entries with each bash item whose sending message is loaded taken out, and those items by message, to draw the two as one row. An item with no such message stays as its own row. */
export function foldBash(entries: TimelineEntry[]): { entries: TimelineEntry[]; bash: Map<number, TranscriptItem> } {
  const sent = new Set(entries.flatMap((e) => (e.kind === 'message' ? [e.message.id] : [])));
  const bash = new Map<number, TranscriptItem>();
  const rest = entries.filter((e) => {
    if (e.kind !== 'item' || e.item.kind !== 'bash' || e.item.request == null || !sent.has(e.item.request)) return true;
    bash.set(e.item.request, e.item);
    return false;
  });
  return { entries: rest, bash };
}

/** One turn's record: a run of tool steps and the agent's own text between them, drawn as one folded line. */
export interface TurnRecord {
  kind: 'record';
  /** The first item's id, so the line keeps its key while the run grows. */
  id: number;
  at: string;
  until: string;
  items: TranscriptItem[];
  /** The commands run, not counting permission asks. */
  steps: number;
}

/** A session's own text (an even reply id) goes in the record; an answer to a request (odd) stays in the chat. */
const recorded = (e: TimelineEntry) => e.kind === 'item' && (isToolStep(e.item) || (e.item.kind === 'reply' && e.item.id % 2 === 0));

/** The entries with each run of tool steps and agent text taken in as one TurnRecord: the chat keeps what the agent said to people. */
export function foldTurns(entries: TimelineEntry[]): (TimelineEntry | TurnRecord)[] {
  const out: (TimelineEntry | TurnRecord)[] = [];
  for (const e of entries) {
    if (!recorded(e) || e.kind !== 'item') {
      out.push(e);
      continue;
    }
    const last = out.at(-1);
    const run: TurnRecord = last?.kind === 'record' ? last : { kind: 'record', id: e.id, at: e.at, until: e.at, items: [], steps: 0 };
    if (run !== last) out.push(run);
    run.items.push(e.item);
    run.until = e.at;
    if (e.item.kind === 'tool') run.steps++;
  }
  return out;
}

/** The daemon's bashWatch (go/internal/watcher/chat.go); after that an item with none is no longer running as far as anyone can tell. */
export const BASH_WATCH_MS = 15 * 60_000;

/** The words under a message whose delivery is unknown. A ! line must not be advised to go again: that runs the command again. */
export const maybeRanNote = (body: string) =>
  body.startsWith('!')
    ? 'It may or may not have run. Look at its terminal before running it again.'
    : 'This message may or may not have run. It is not sent again; send it again yourself if it matters.';

const PLAIN_START = new Set(['launch', 'start']);
/** One restart is one line: back-to-back session starts collapse into the last, which keeps the handoff and the cause the run was told (a plain launch is the least telling cause). */
function mergeSessions(a: Extract<TimelineEntry, { kind: 'item' }>, b: Extract<TimelineEntry, { kind: 'item' }>): TimelineEntry {
  const [x, y] = [a.item.session, b.item.session];
  const cause = [y?.cause, x?.cause].find((c) => c && !PLAIN_START.has(c)) ?? y?.cause ?? x?.cause;
  const changed = [...new Set([x?.changed, y?.changed].filter(Boolean))].join(', '); // the same reason twice is said once
  const session = { ...y!, handoff: y?.handoff || x?.handoff || null, ...(cause ? { cause } : {}), ...(changed ? { changed } : {}) };
  return { ...b, item: { ...b.item, session } };
}

/** The name the daemon records for the person's own words: the CLI's `human`, or the device they typed on (`me`, e.g. "Handline desktop"). */
export const isPerson = (name: string, me: string) => name === 'human' || (!!me && name === me);
/** A key that names a person, not an agent: people.list keys are `p<n>`; `human` is the founder's older name. */
export const isPersonKey = (key: string | null | undefined): boolean => key === 'human' || (!!key && /^p\d+$/.test(key));
/** One line of Markdown as plain words, for a one-line preview: emphasis, code ticks, heading and quote marks and link targets dropped. */
export const plainLine = (text: string): string =>
  text
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/`([^`\n]+)`/g, '$1')
    .replace(/(?<!\w)(\*\*|__|\*|_|~~)(?=\S)([^\n]*?\S)\1(?!\w)/g, '$2')
    .replace(/^\s*(#{1,6}|>|[-*+]|\d+\.)\s+/gm, '')
    .replace(/\s+/g, ' ')
    .trim();
/** A task handed to an agent: the board sends it in the asker's name, when the agent reaches it, so it is the task's delivery and not words the person wrote then. */
export const isTaskDelivery = (m: ChatMessage) => m.kind === 'task' && !!m.tasks?.length;
/** Who a message is from, for the agent whose chat this is: the person, the agent, or someone else (another agent, the task board: the *all* tab, notices). Only the person's own words sit on the person's side. */
export function sideOf(m: ChatMessage, agent: string, me: string): 'person' | 'agent' | 'other' {
  if (m.from === agent) return 'agent';
  if (isTaskDelivery(m)) return 'other';
  return isPerson(m.from, me) ? 'person' : 'other';
}

/** Queued behind a running turn: the agent has not been given it. */
export const waiting = (m: ChatMessage) => (m.receipt ? m.receipt.state === 'queued' : m.landing === 'queued');

/** How far the person's own message got, for the two ticks: 0 not sent yet, 1 sent and not read, 2 read. */
export function tickLevel(m: ChatMessage): 0 | 1 | 2 {
  if (m.id < 0 || waiting(m)) return 0;
  if (m.receipt) return m.receipt.state === 'confirmed' ? 2 : 1;
  return m.landing === 'typed' ? 1 : 2;
}

/** A task its last blocker freed, as its start card says it: `Started: T8632, its last blocker, is done`. Null for any other item. */
export function unblockedStart(i: TranscriptItem): { task: number; detail: string } | null {
  const e = i.task_event;
  if (e?.event !== 'unblocked') return null;
  return { task: e.task, detail: `Started: ${e.closed_task ? `T${e.closed_task}, its last blocker,` : 'its last blocker'} is done` };
}

const HANDOFF_WHY: Record<string, string> = {
  context_full: 'Context full',
  move: 'Moving to another account',
  usage: 'Usage limit',
  login: 'Login needed',
  restart: 'Restart asked',
};
type HandoffPhase = 'waiting' | 'writing' | 'starting';
const HANDOFF_WHAT = { waiting: 'Waiting for its turn to end', writing: 'Writing the handoff', starting: 'Starting a new session' };
/** The live line while a session hands off or restarts: why it ends (the cause is an open set, so an unknown one is shown as it came; changed settings are named) and what is happening now. */
export function handoffLine(h: { cause: string; phase: HandoffPhase; settings?: string[] }): { why: string; what: string } {
  const changed = h.cause === 'settings_applied' ? `${h.settings?.length ? h.settings.join(', ') : 'settings'} changed` : '';
  return {
    why: changed ? changed.charAt(0).toUpperCase() + changed.slice(1) : (HANDOFF_WHY[h.cause] ?? h.cause),
    what: HANDOFF_WHAT[h.phase] ?? HANDOFF_WHAT.starting,
  };
}

/** The live restart line, sidebar and chat alike, what happens now first: `Waiting for its turn to end, then restarts: model changed`. */
export function restartingLine(h: { cause: string; phase: HandoffPhase; settings?: string[] }): string {
  const { why, what } = handoffLine(h);
  return `${what}${h.phase === 'waiting' ? ', then restarts' : ' to restart'}: ${why.charAt(0).toLowerCase()}${why.slice(1)}`;
}

/** `12m` for a turn that began at `since`, `<1m` before the first minute. */
export function turnMinutes(since: string | null, now = Date.now()): string {
  const m = since ? Math.floor((now - Date.parse(since)) / 60_000) : 0;
  return m < 1 ? '<1m' : `${m}m`;
}

const GIST: [RegExp, string][] = [
  [/^(sudo\s+)?cp\b/, 'Copies files'],
  [/^(sudo\s+)?mv\b/, 'Moves files'],
  [/^(sudo\s+)?rm\b/, 'Deletes files'],
  [/^mkdir\b/, 'Makes a folder'],
  [/^git\s+push\b/, 'Pushes commits'],
  [/^git\s+commit\b/, 'Makes a commit'],
  [/^git\s+(checkout|switch)\b/, 'Switches branch'],
  [/^(pnpm|npm|yarn)\s+(run\s+)?test\b|^go\s+test\b|^pytest\b|^vitest\b/, 'Runs tests'],
  [/^(pnpm|npm|yarn)\s+(install|add|i)\b|^go\s+get\b|^pip\s+install\b/, 'Installs packages'],
  [/^curl\b|^wget\b/, 'Fetches from the network'],
];
/** What a shell command does, in a few plain words, when its first word says so plainly; else null, never guessed. */
export function commandGist(command: string | null | undefined): string | null {
  const c = (command ?? '').trim().replace(/^(cd\s+\S+\s*&&\s*)+/, '');
  return GIST.find(([re]) => re.test(c))?.[1] ?? null;
}
