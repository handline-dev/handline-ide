// The fake daemon's task comments: the comments on a row, the asks to the founder they raise, and who was told.
import type { Comment, CommentFile, CommentReader } from '@handline/contract';
import { refs } from '#src/attachments.ts';
import { TaskFailure, TaskStore, type Row } from '#src/taskstore.ts';
import { FOUNDER } from '#src/founder.ts';

/** Why task.comment.recipients names an agent. */
type Why = 'asked' | 'assignee' | 'owner' | 'last_writer';

/** A comment written to the founder, as the daemon reads it: it opens with the founder's mention (addr.Lead). */
const TO_FOUNDER = /^\s*<@human>/;

/** An `@name` in a comment, as the daemon reads it: at the start or after a character that is not part of a name. */
const NAMED = /(?:^|[^A-Za-z0-9._@-])@([A-Za-z0-9][A-Za-z0-9._-]*)/g;

export interface Pending {
  task: number | null;
  title: string;
  kind: 'allow' | 'decide' | 'ask' | 'warning' | 'comment' | 'note' | 'run';
  source?: 'task' | 'comment' | 'prompt' | 'proposal' | 'machine';
  id?: string;
  agent?: string | null;
  ask?: string | null;
  warning?: {
    kind: 'disk' | 'login' | 'delivery' | 'undelivered' | 'notice' | 'review';
    account?: string;
    agent?: string;
    line?: string;
    level?: 'one_in_two' | 'one_in_three' | 'one_in_five' | 'never';
    work?: 'contract' | 'screen' | 'daemon' | 'docs';
    stall?: boolean;
  } | null;
  items: number[];
  /** The open tasks that wait on this row's task. */
  holds?: number[];
  unread: number;
  first_unread: number | null;
  last_at: string;
}

export class TaskComments extends TaskStore {
  /** The daemon's own warnings in requests.pending (a login gone, weekly use about to be lost); a seed puts them in. */
  readonly warnings: Pending[] = [];
  /** Drafts only the founder can run (queued work waits on them, or their agent left them); a seed puts them in. */
  readonly runs: Pending[] = [];
  /** What waits on a task, by task: a seed puts it in, and a founder's turn row on that task carries it. */
  readonly holds = new Map<number, number[]>();
  /** The warnings the founder dismissed, by id. */
  private readonly dismissed = new Set<string>();

  /**
   * task.comment.recipients, by the daemon's one rule: a comment naming agents with `@` is told to them only (asked). One naming no one goes
   * to the holder (assignee), and to who the task answers to (owner) when sent now or written by the founder on a task no agent holds; the
   * founder's on a task they hold goes to the last agent that commented. The founder's answer to their turn also reaches the holder below
   * it, whomever it names. The founder's naming no one on an agent's draft with no turn goes to the agent that opened it only. Never the writer.
   */
  recipients(p: Record<string, unknown>, person: string): { recipients: { agent: string; why: Why }[] } {
    const t = this.find(p.task);
    const founder = !this.isAgent(person);
    const top = t.turns?.at(-1);
    const back = founder && top?.agent === FOUNDER;
    const acting = back ? top.by : (t.acting ?? t.to);
    const named = [...String(p.text).matchAll(NAMED)]
      .map((m) => m[1]!.replace(/[.-]+$/, ''))
      .filter((n) => this.isAgent(n) && n !== person);
    if (named.length === 0 && founder && !back && this.unsentDraft(t)) {
      const opener = this.draftOpener(t);
      return { recipients: opener ? [{ agent: opener, why: 'owner' }] : [] };
    }
    const last =
      named.length === 0 && founder && acting === person
        ? ([...(t.comments ?? [])].reverse().find((m) => m.author !== person && this.isAgent(m.author))?.author ?? '')
        : '';
    const listed = named.length === 0 ? [t.from, acting] : back ? [acting] : [];
    const unheld = founder && !this.isAgent(acting);
    const seen = new Set<string>(['', person, 'human', FOUNDER]);
    const recipients: { agent: string; why: Why }[] = [];
    for (const agent of [...listed, ...named, last]) {
      if (seen.has(agent)) continue;
      seen.add(agent);
      const asked = named.includes(agent);
      if (!(p.now === true || asked || agent === acting || agent === last || unheld)) continue;
      recipients.push({ agent, why: asked ? 'asked' : agent === last ? 'last_writer' : agent === acting ? 'assignee' : 'owner' });
    }
    return { recipients };
  }

  /** task.seen: the marker moves forward to `upto`, by default the task's newest comment, and never back; a task with no comments is marked too. */
  seen(p: Record<string, unknown>): void {
    const t = this.find(p.task);
    const newest = (t.comments ?? []).reduce((n, m) => Math.max(n, m.id), 0);
    const upto = typeof p.upto === 'number' ? p.upto : newest;
    if (upto > (this.seenUpto.get(t.id) ?? -1)) {
      this.seenUpto.set(t.id, upto);
      this.emit(this.view(t));
    }
    this.host.requests.markSeen(t.id);
  }

  /** warning.dismiss: the warning leaves requests.pending; false when there is no warning by that id. A second dismissal is accepted. */
  dismissWarning(id: string): boolean {
    const w = this.warnings.find((x) => x.id === id);
    if (!w) return false;
    if (!this.dismissed.has(id)) this.host.emit('for_you', 'for_you.closed', { title: w.title, data: { kind: 'machine', id } });
    this.dismissed.add(id);
    return true;
  }

  /** requests.pending: what waits on the founder by task, and the counts the badges show. */
  pending(): {
    answer: Pending[];
    comments: Pending[];
    counts: { answer: number; notes: number; comments: number; needs_you: number; drafts: number; unseen_results: number };
  } {
    const strength = { note: 0, comment: 0, run: 0, warning: 1, ask: 2, decide: 3, allow: 4 };
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
      if (t.state === 'done' || t.state === 'discarded' || (t.state === 'draft' && t.to === 'human')) continue;
      const top = t.turns?.at(-1);
      const v = this.view(t);
      if (top?.agent === 'p1' && !answer.has(`task:${t.id}`))
        answer.set(`task:${t.id}`, {
          task: t.id,
          title: t.title,
          kind: t.state === 'draft' ? 'run' : 'decide', // a draft its opener asks on: Run is one answer
          source: 'task',
          id: String(t.id),
          agent: top.by,
          ask: top.ask.split('\n')[0] ?? null,
          holds: this.holds.get(t.id) ?? [],
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
    // What holds work up comes first, the longest wait first, as the daemon orders it.
    const holdsFirst = (a: Pending, b: Pending) => {
      const ha = (a.holds ?? []).length > 0;
      const hb = (b.holds ?? []).length > 0;
      if (ha !== hb) return ha ? -1 : 1;
      return ha ? a.last_at.localeCompare(b.last_at) : newest(a, b);
    };
    const runs = this.runs.filter((r) => r.task === null || (!answer.has(`task:${r.task}`) && this.runnable(r.task, r.holds ?? [])));
    const shown = this.warnings.filter((w) => !this.dismissed.has(w.id!));
    const open = this.rows.filter((r) => r.state !== 'done' && r.state !== 'discarded');
    return {
      answer: [...answer.values(), ...runs, ...shown, ...notes].map((r) => ({ ...r, holds: r.holds ?? [] })).sort(holdsFirst),
      comments: comments.sort(newest),
      counts: {
        answer: answer.size + runs.length + shown.length,
        notes: notes.length,
        comments: comments.length,
        needs_you: open.filter((r) => r.state !== 'draft' && (r.to === 'human' || this.waiting(r)?.on === 'founder')).length,
        drafts: open.filter((r) => r.state === 'draft').length,
        unseen_results: this.host.requests.unseen().length,
      },
    };
  }

  /** As the daemon: a run row stands while its task is a draft, and one nothing waits on (left behind) until the founder sees it at its newest comment. */
  private runnable(id: number, holds: number[]): boolean {
    const t = this.rows.find((r) => r.id === id);
    if (t?.state !== 'draft') return false;
    const newest = (t.comments ?? []).reduce((n, m) => Math.max(n, m.id), 0);
    return holds.length > 0 || (this.seenUpto.get(id) ?? -1) < newest;
  }

  /** As the daemon: the newest comment to the founder by anyone but the founder, unread on its task and in its author's chat, on a task no row
   * stands for, is a note. */
  private notes(answer: Map<string, Pending>): Pending[] {
    const notes: Pending[] = [];
    for (const t of this.rows) {
      if (answer.has(`task:${t.id}`) || t.state === 'discarded') continue;
      const v = this.view(t);
      const m = (t.comments ?? [])
        .filter((x) => x.author !== 'human' && x.id > (this.seenUpto.get(t.id) ?? 0) && TO_FOUNDER.test(x.text))
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
        ask: m.text.replace(TO_FOUNDER, '').trim().split('\n')[0]!,
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
    return (t.comments ?? []).reduce((at, m) => (m.at > at ? m.at : at), t.created);
  }

  comment(p: Record<string, unknown>, op: 'add' | 'edit' | 'delete', person: string, agent = false): number {
    const t = this.find(p.task);
    const comments = (t.comments ??= []);
    if (op === 'add') {
      this.images(String(p.text), '', 'text', agent);
      const id = comments.reduce((n, m) => Math.max(n, m.id), 0) + 1;
      const files = Array.isArray(p.files) ? p.files.map((f) => this.host.uploads.commentFile(String(f))) : [];
      if (p.reply !== undefined && !comments.some((m) => m.id === p.reply))
        throw new TaskFailure('refused', `comment ${String(p.reply)} does not exist`);
      comments.push({
        id,
        task: t.id,
        author: person,
        at: this.now(),
        text: String(p.text),
        ...(p.reply === undefined ? {} : { reply_to: Number(p.reply) }),
        ...(files.length ? { files } : {}),
      });
      (t.history ??= []).push({ at: this.now(), by: person, field: 'comment', was: '', now: String(p.text), comment: id });
      this.askFor(t, comments[comments.length - 1]!, true);
      return id;
    }
    const i = comments.findIndex((m) => m.id === p.comment);
    if (i < 0) throw new TaskFailure('refused', `comment ${String(p.comment)} does not exist`);
    if (op === 'edit') {
      this.images(String(p.text), comments[i]!.text, 'text', agent);
      (t.history ??= []).push({
        at: this.now(),
        by: person,
        field: 'comment',
        was: comments[i]!.text,
        now: String(p.text),
        comment: comments[i]!.id,
      });
      comments[i] = { ...comments[i]!, text: String(p.text) };
      this.askFor(t, comments[i]!, false);
    } else {
      (t.history ??= []).push({ at: this.now(), by: person, field: 'comment', was: comments[i]!.text, now: '', comment: comments[i]!.id });
      this.askFor(t, comments[i]!, false, true);
      comments.splice(i, 1);
    }
    return comments[i]?.id ?? 0;
  }

  /**
   * The ask to the founder that comment `m` raised, kept as the comment reads now: an ask an agent's comment to the founder opened before comments stopped asking stays open.
   * The founder writing on the task answers it.
   */
  protected askFor(t: Row, m: Comment, adding: boolean, gone = false): void {
    const inbox = this.host.inbox;
    if (!this.isAgent(m.author)) return adding ? inbox.closeAsks(t.id) : undefined;
    const open = inbox.askOf(t.id, m.id);
    const want = !gone && TO_FOUNDER.test(m.text);
    if (want && open) inbox.retext(open, m.text);
    else if (open) inbox.close(open);
  }

  /** Where the tell of a founder's comment stands for each agent it was told to, as the daemon reports it (Comment.readers). */
  setReaders(task: number, comment: number, readers: CommentReader[]): void {
    const m = this.find(task).comments?.find((x) => x.id === comment);
    if (!m) throw new TaskFailure('refused', `comment ${comment} does not exist`);
    m.readers = readers;
  }

  /** A comment that carries files, as `hl task comment add --file` leaves it. */
  commentWithFiles(task: number, author: string, text: string, files: CommentFile[]): number {
    const id = this.comment({ task, text }, 'add', author);
    const m = this.find(task).comments!.find((x) => x.id === id)!;
    m.files = files;
    return id;
  }
  /** How many briefs and comments name image id, or carry it as a comment's file: attachment.remove's count. */
  naming(id: string): number {
    let n = 0;
    for (const t of this.rows) {
      if (refs(t.body).includes(id)) n++;
      for (const m of t.comments ?? []) if (refs(m.text).includes(id) || m.files?.some((f) => f.id === id)) n++;
    }
    return n;
  }
  /** Every comment's files through mark, so one the founder removed reads gone and removed. */
  remark(mark: (f: CommentFile) => CommentFile): void {
    for (const t of this.rows) for (const m of t.comments ?? []) if (m.files) m.files = m.files.map(mark);
  }
}
