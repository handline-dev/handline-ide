// The fake daemon's requests: the founder's requests.list rows, all tasks, and task.request, which turns a message of theirs into one. A test
// or a seed puts rows in with `add`; the requests topic says when one changed.
import type { RequestRow, Timestamp } from '@handline/contract';
import { Failure, invalidParams } from '#src/errors.ts';

interface Host {
  emit(topic: 'requests', type: string, f: { agent?: string; data: Record<string, unknown> }): void;
  /** The chat, for a request named by the number of a message of the founder's. */
  chat: {
    find(id: unknown): { id: number; to: string; body: string; agent: string };
    change(m: never, patch: { request_task?: number | null }): void;
  };
}
type Accepted = (said: string, extra?: object) => unknown;

const METHODS = /^(requests.list|task.request)$/;
const PAGE = 50;

export class FakeRequests {
  readonly rows: RequestRow[] = [];
  private host: Host;
  private now: () => Timestamp;
  private next = 9000;

  constructor(host: Host, now: () => Timestamp) {
    this.host = host;
    this.now = now;
  }

  owns = (name: string): boolean => METHODS.test(name);
  lists = (): boolean => true;

  /** A request row as the daemon reads it; what a test does not say is the plainest value. */
  add(row: Partial<RequestRow> & Pick<RequestRow, 'title'>): RequestRow {
    const full: RequestRow = {
      kind: 'task',
      task: row.task ?? this.next++,
      ref: null,
      status: 'doing',
      agent: 'handline.architect',
      holders: [],
      open: 0,
      done: 0,
      prs: [],
      now: null,
      blocked: false,
      progress: '',
      unread_comments: 0,
      created: this.now(),
      last_activity: this.now(),
      cursor: `c${String(9999 - this.rows.length).padStart(4, '0')}`,
      ...row,
    };
    if (full.now === null && full.status !== 'done') full.now = full.holders[0] ?? null;
    if (row.progress === undefined) full.progress = progressLine(full);
    this.rows.unshift(full);
    this.host.emit('requests', 'request.changed', { agent: full.agent, data: { ...full } });
    return full;
  }

  call(method: string, p: Record<string, unknown>, accepted: Accepted): unknown {
    if (method === 'requests.list') return this.list(p);
    const m = this.message(p.request);
    if (!m) throw new Failure('not_found', `no message ${String(p.request)} of the founder's`, { field: 'request' });
    const ref = `msg:${m.id}`;
    const made = this.rows.find((r) => r.ref === ref);
    if (made) return accepted('Already a request.', { task: made.task, existing: true });
    const title = typeof p.title === 'string' && p.title.trim() ? p.title.trim() : m.body.split('\n')[0]!.slice(0, 80);
    const row = this.add({ title, ref, agent: m.to, holders: [m.to], origin: m.to });
    this.host.chat.change(m as never, { request_task: row.task });
    return accepted('Request made.', { task: row.task });
  }

  /** requests.pending's unseen_results: the finished rows the founder has not seen. */
  unseen = (): RequestRow[] => this.rows.filter((r) => r.ended?.seen === false);

  /** task.seen: the row of an ended task is seen. */
  markSeen(task: number): void {
    const row = this.rows.find((r) => r.task === task && r.ended && !r.ended.seen);
    if (!row?.ended) return;
    row.ended.seen = true;
    this.host.emit('requests', 'request.changed', { agent: row.agent, data: { ...row } });
  }

  /** The founder's chat message a request names by its number. */
  private message(name: unknown): { id: number; to: string; body: string } | undefined {
    if (typeof name !== 'number' && !/^\d+$/.test(String(name))) return undefined;
    try {
      return this.host.chat.find(Number(name));
    } catch {
      return undefined;
    }
  }

  private list(p: Record<string, unknown>) {
    const limit = typeof p.limit === 'number' ? p.limit : PAGE;
    const from = typeof p.before === 'string' ? this.rows.findIndex((r) => r.cursor === p.before) + 1 : 0;
    if (p.unseen !== undefined && typeof p.unseen !== 'boolean') throw invalidParams('unseen is true or false', 'unseen');
    const kept = this.rows.filter((r) => (p.state === undefined || r.status === p.state) && (p.unseen !== true || r.ended?.seen === false));
    return { items: kept.slice(from, from + limit), more: kept.length > from + limit };
  }
}

/** The one-line progress of a request: the same rule as the daemon's progressLine (go/internal/reads/requests.go). */
export function progressLine(r: Pick<RequestRow, 'status' | 'open' | 'done' | 'now' | 'blocked'>): string {
  if (r.status === 'done') return 'done';
  const parts: string[] = [];
  if (r.open + r.done > 0) parts.push(`sub ${r.done}/${r.open + r.done} done`);
  if (r.now) parts.push(`now ${r.now}`);
  if (r.status === 'waiting_on_you') parts.push('waiting on you');
  else if (r.blocked) parts.push('blocked');
  else if (r.status === 'review') parts.push('in review');
  return parts.join(', ');
}
