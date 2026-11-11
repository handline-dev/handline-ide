// The fake daemon's task comments: the memos on a row, the asks to the founder they raise, and who was told.
import type { Memo, MemoFile, MemoReader } from '@handline/contract';
import { refs } from '#src/attachments.ts';
import { TaskFailure, TaskStore, type Row } from '#src/taskstore.ts';

/** `@human` in a comment: at the start or after a character that is not part of a name. */
const NAMES_HUMAN = /(?:^|[^A-Za-z0-9._@-])@human(?:[^A-Za-z0-9_-]|$)/;

/** An `@name` in a comment, as the daemon reads it: at the start or after a character that is not part of a name. */
const NAMED = /(?:^|[^A-Za-z0-9._@-])@([A-Za-z0-9][A-Za-z0-9._-]*)/g;

export interface Pending {
  task: number | null;
  title: string;
  kind: 'allow' | 'decide' | 'ask' | 'warning' | 'comment' | 'note';
  source?: 'task' | 'comment' | 'prompt' | 'proposal' | 'machine';
  id?: string;
  agent?: string | null;
  ask?: string | null;
  warning?: { kind: 'disk' | 'login' | 'delivery' | 'undelivered'; account?: string; agent?: string } | null;
  items: number[];
  unread: number;
  first_unread: number | null;
  last_at: string;
}

export class TaskComments extends TaskStore {
  /** The daemon's own warnings in requests.pending (a login gone, weekly use about to be lost); a seed puts them in. */
  readonly warnings: Pending[] = [];

  /**
   * task.comment.recipients: who the daemon tells of a comment, and why, by its one rule. An agent named with `@` is asked and the assignee (the
   * holder) is always told; who the task answers to is told when it is sent now, or when the founder wrote it on a task no agent holds. Never the writer.
   */
  recipients(p: Record<string, unknown>, person: string): { recipients: { agent: string; why: 'asked' | 'assignee' | 'owner' }[] } {
    const t = this.find(p.task);
    const named = [...String(p.text).matchAll(NAMED)].map((m) => m[1]!.replace(/[.-]+$/, '')).filter((n) => this.isAgent(n));
    const aimed = p.now === true || (!this.isAgent(person) && !this.isAgent(t.to));
    const seen = new Set<string>(['', person, 'human']);
    const recipients: { agent: string; why: 'asked' | 'assignee' | 'owner' }[] = [];
    for (const agent of [t.from, t.to, ...named]) {
      if (seen.has(agent)) continue;
      seen.add(agent);
      const asked = named.includes(agent);
      if (!asked && !aimed && agent !== t.to) continue;
      recipients.push({ agent, why: asked ? 'asked' : agent === t.to ? 'assignee' : 'owner' });
    }
    return { recipients };
  }

  /** task.seen: the marker moves forward to `upto`, by default the task's newest comment, and never back. */
  seen(p: Record<string, unknown>): void {
    const t = this.find(p.task);
    const newest = (t.memos ?? []).reduce((n, m) => Math.max(n, m.id), 0);
    const upto = typeof p.upto === 'number' ? p.upto : newest;
    if (upto > (this.seenUpto.get(t.id) ?? 0)) {
      this.seenUpto.set(t.id, upto);
      this.emit(this.view(t));
    }
    this.host.requests.markSeen(t.id);
  }

  /** requests.pending: what waits on the founder by task, and the counts the badges show. */
  pending(): {
    answer: Pending[];
    comments: Pending[];
    counts: { answer: number; notes: number; comments: number; needs_you: number; drafts: number; unseen_results: number };
  } {
    const strength = { note: 0, comment: 0, warning: 1, ask: 2, decide: 3, allow: 4 };
    const answer = new Map<string, Pending>();
    for (const i of this.host.inbox.list({ unread: true, limit: 500 }).items) {
      const kind = i.kind === 'ask' ? 'ask' : i.kind === 'decision' ? 'decide' : i.kind === 'prompt' ? 'allow' : null;
      if (!kind) continue;
      const key = i.task === null ? `item:${i.id}` : `task:${i.task}`;
      const row = answer.get(key);
      if (row) {
        row.items.push(i.id);
        if (strength[kind] > strength[row.kind]) row.kind = kind;
        if (i.created > row.last_at) row.last_at = i.created;
        continue;
      }
      const t = i.task === null ? null : this.rows.find((r) => r.id === i.task);
      const v = t ? this.view(t) : null;
      answer.set(key, {
        task: i.task,
        title: t?.title ?? i.title,
        kind,
        source: i.task === null ? (i.kind === 'prompt' ? 'prompt' : 'comment') : 'task',
        id: String(i.task ?? i.id),
        agent: i.agent,
        ask: null,
        warning: null,
        items: [i.id],
        unread: v?.unread ?? 0,
        first_unread: v?.first_unread ?? null,
        last_at: i.created,
      });
    }
    // A proposal waiting for the founder is its own row to allow, as the daemon lists it (open or apply_failed, never applying).
    for (const p of this.host.script.proposals.proposals.filter((x) => x.state === 'open' || x.state === 'apply_failed'))
      answer.set(`proposal:${p.id}`, {
        task: null,
        title: p.title,
        kind: 'allow',
        source: 'proposal',
        id: String(p.id),
        agent: p.agent,
        ask: null,
        warning: null,
        items: [],
        unread: 0,
        first_unread: null,
        last_at: p.created,
      });
    // The founder's turn on a task is a row to decide, its ask the first line of what was asked.
    for (const t of this.rows) {
      if (t.state === 'done' || t.state === 'discarded') continue;
      const top = t.turns?.at(-1);
      const v = this.view(t);
      if (top?.agent === 'human' && !answer.has(`task:${t.id}`))
        answer.set(`task:${t.id}`, {
          task: t.id,
          title: t.title,
          kind: 'decide',
          source: 'task',
          id: String(t.id),
          agent: top.by,
          ask: top.ask.split('\n')[0] ?? null,
          warning: null,
          items: [],
          unread: v.unread ?? 0,
          first_unread: v.first_unread ?? null,
          last_at: top.since,
        });
    }
    const notes = this.notes(answer);
    const noted = new Set(notes.map((n) => n.task));
    const comments: Pending[] = [];
    for (const t of this.rows) {
      if (noted.has(t.id)) continue;
      const v = this.view(t);
      const mine = t.from === 'human' || t.to === 'human';
      if (mine && (v.unread ?? 0) > 0 && !answer.has(`task:${t.id}`) && t.state !== 'discarded')
        comments.push({
          task: t.id,
          title: t.title,
          kind: 'comment',
          items: [],
          unread: v.unread ?? 0,
          first_unread: v.first_unread ?? null,
          last_at: this.lastComment(t),
        });
    }
    const newest = (a: Pending, b: Pending) => b.last_at.localeCompare(a.last_at);
    const open = this.rows.filter((r) => r.state !== 'done' && r.state !== 'discarded');
    return {
      answer: [...answer.values(), ...this.warnings, ...notes].sort(newest),
      comments: comments.sort(newest),
      counts: {
        answer: answer.size + this.warnings.length,
        notes: notes.length,
        comments: comments.length,
        needs_you: open.filter((r) => r.state !== 'draft' && (r.to === 'human' || this.waiting(r)?.on === 'founder')).length,
        drafts: open.filter((r) => r.state === 'draft').length,
        unseen_results: this.host.requests.unseen().length,
      },
    };
  }

  /** As the daemon: the newest `@human` comment by anyone but the founder, unread on its task and in its author's chat, on a task no row
   * stands for, is a note. */
  private notes(answer: Map<string, Pending>): Pending[] {
    const notes: Pending[] = [];
    for (const t of this.rows) {
      if (answer.has(`task:${t.id}`) || t.state === 'discarded') continue;
      const v = this.view(t);
      const m = (t.memos ?? [])
        .filter((x) => x.author !== 'human' && x.id > (this.seenUpto.get(t.id) ?? 0) && NAMES_HUMAN.test(x.text))
        .at(-1);
      const seen = m && this.host.chat.seenAt(m.author);
      if (!m || (seen && seen >= m.at)) continue; // seen as a card in its author's chat
      notes.push({
        task: t.id,
        title: t.title,
        kind: 'note',
        source: 'comment',
        id: String(m.id),
        agent: m.author,
        ask: m.text.replace(NAMES_HUMAN, ' ').trim().split('\n')[0]!,
        warning: null,
        items: [],
        unread: v.unread ?? 0,
        first_unread: v.first_unread ?? null,
        last_at: m.at,
      });
    }
    return notes;
  }

  private lastComment(t: Row): string {
    return (t.memos ?? []).reduce((at, m) => (m.at > at ? m.at : at), t.created);
  }

  memo(p: Record<string, unknown>, op: 'add' | 'edit' | 'delete', person: string, agent = false): number {
    const t = this.find(p.task);
    const memos = (t.memos ??= []);
    if (op === 'add') {
      this.images(String(p.text), '', 'text', agent);
      const id = memos.reduce((n, m) => Math.max(n, m.id), 0) + 1;
      const files = Array.isArray(p.files) ? p.files.map((f) => this.host.uploads.memoFile(String(f))) : [];
      if (p.reply !== undefined && !memos.some((m) => m.id === p.reply))
        throw new TaskFailure('refused', `memo ${String(p.reply)} does not exist`);
      memos.push({
        id,
        task: t.id,
        author: person,
        at: this.now(),
        text: String(p.text),
        ...(p.reply === undefined ? {} : { reply_to: Number(p.reply) }),
        ...(files.length ? { files } : {}),
      });
      (t.history ??= []).push({ at: this.now(), by: person, field: 'memo', was: '', now: String(p.text), memo: id });
      this.askFor(t, memos[memos.length - 1]!, true);
      return id;
    }
    const i = memos.findIndex((m) => m.id === p.memo);
    if (i < 0) throw new TaskFailure('refused', `memo ${String(p.memo)} does not exist`);
    if (op === 'edit') {
      this.images(String(p.text), memos[i]!.text, 'text', agent);
      (t.history ??= []).push({ at: this.now(), by: person, field: 'memo', was: memos[i]!.text, now: String(p.text), memo: memos[i]!.id });
      memos[i] = { ...memos[i]!, text: String(p.text) };
      this.askFor(t, memos[i]!, false);
    } else {
      (t.history ??= []).push({ at: this.now(), by: person, field: 'memo', was: memos[i]!.text, now: '', memo: memos[i]!.id });
      this.askFor(t, memos[i]!, false, true);
      memos.splice(i, 1);
    }
    return memos[i]?.id ?? 0;
  }

  /**
   * The ask to the founder that comment `m` raised, kept as the comment reads now: an ask an agent's `@human` comment opened before comments stopped asking stays open.
   * The founder writing on the task answers it.
   */
  protected askFor(t: Row, m: Memo, adding: boolean, gone = false): void {
    const inbox = this.host.inbox;
    if (!this.isAgent(m.author)) return adding ? inbox.closeAsks(t.id) : undefined;
    const open = inbox.askOf(t.id, m.id);
    const want = !gone && NAMES_HUMAN.test(m.text);
    if (want && open) inbox.retext(open, m.text);
    else if (open) inbox.close(open);
  }

  /** Where the tell of a founder's comment stands for each agent it was told to, as the daemon reports it (Memo.readers). */
  setReaders(task: number, memo: number, readers: MemoReader[]): void {
    const m = this.find(task).memos?.find((x) => x.id === memo);
    if (!m) throw new TaskFailure('refused', `memo ${memo} does not exist`);
    m.readers = readers;
  }

  /** task.comment.send_now: a held tell goes to the agent now. */
  sendNow(p: Record<string, unknown>): void {
    const row = this.rows.find((t) => t.memos?.some((m) => m.id === p.memo && m.readers?.some((r) => r.agent === p.agent)));
    const tell = row?.memos?.find((m) => m.id === p.memo)?.readers?.find((r) => r.agent === p.agent);
    if (!row || !tell || tell.state !== 'held') throw new TaskFailure('refused', 'that comment is not held for this agent');
    Object.assign(tell, { state: 'sent', at: this.now() });
    delete tell.minutes;
    this.emit(row);
  }

  /** A memo that carries files, as `hl task comment add --file` leaves it. */
  memoWithFiles(task: number, author: string, text: string, files: MemoFile[]): number {
    const id = this.memo({ task, text }, 'add', author);
    const m = this.find(task).memos!.find((x) => x.id === id)!;
    m.files = files;
    return id;
  }
  /** How many briefs and memos name image id, or carry it as a memo's file: attachment.remove's count. */
  naming(id: string): number {
    let n = 0;
    for (const t of this.rows) {
      if (refs(t.body).includes(id)) n++;
      for (const m of t.memos ?? []) if (refs(m.text).includes(id) || m.files?.some((f) => f.id === id)) n++;
    }
    return n;
  }
  /** Every memo's files through mark, so one the founder removed reads gone and removed. */
  remark(mark: (f: MemoFile) => MemoFile): void {
    for (const t of this.rows) for (const m of t.memos ?? []) if (m.files) m.files = m.files.map(mark);
  }

  /** The task's comments, each cut at the images its text names (one page: the fake keeps no more). */
  comments(p: Record<string, unknown>, agent = false): { memos: Memo[]; more: boolean } {
    return { memos: this.get(p, agent).memos, more: false };
  }
}
