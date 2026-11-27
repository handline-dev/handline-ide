// The fake daemon's tasks: the rows `tasks.list` and `task.get` answer and the writes that change them, on the tasks topic.
// A task the founder asks for starts as `queued`; withdrawing or cancelling one that has not been delivered discards it.
import { invalidParams } from '#src/errors.ts';
import { fieldsOf } from '#src/history.ts';
import { TaskFailure, type Row } from '#src/taskstore.ts';
import { TaskReviews } from '#src/taskreviews.ts';

import type { Lesson } from '@handline/contract';

export { TaskFailure };
export type ReviewLevel = 'every' | 'one_in_two' | 'one_in_three' | 'one_in_five' | 'never';
export type ReviewWork = 'contract' | 'screen' | 'daemon' | 'docs';
export const REVIEW_LEVELS: ReviewLevel[] = ['every', 'one_in_two', 'one_in_three', 'one_in_five', 'never'];
export const REVIEW_WORKS: ReviewWork[] = ['contract', 'screen', 'daemon', 'docs'];
type Rung = { level: ReviewLevel; record: { task: number; ok_first: boolean; at: string }[] };

/** The headings the daemon's launch prompt puts over an agent's lessons and a person's (db.Section), counted in their bytes. */
const LESSONS_HEADING = "# What you learned (your lessons)\nFrom the founder's Redo notes. Apply them to every task.";
const FOUNDER_HEADING =
  "# The founder's preferences\nWhat the founder said they want, kept by the agents who work for them. Follow them on every task.";

export class FakeTasks extends TaskReviews {
  /** The methods this fake answers: the basic ones, and the verbs of the daemon's plan (D3) only when asked for. */
  serves(method: string): boolean {
    return BASIC.has(method) || (this.verbs && VERBS.has(method));
  }
  /** Whether hello lists the method: every method that is not a task one, and the task ones this serves. */
  lists(method: string): boolean {
    return !/^tasks?\./.test(method) || this.serves(method) || /^task\.(request|none|send_now)$/.test(method); // served by the requests and admin modules
  }
  /** lessons.list: an agent's live lessons and the bytes the launch prompt's section takes. */
  lessonsOf(agent: string): { agent: string; items: Lesson[]; bytes: number; cap: number } {
    const items = this.lessons.get(agent) ?? [];
    const lines = items.map((l) => `\n- ${l.text}${l.task ? ` (T${l.task})` : ''}`).join('');
    const head = /^p\d+$/.test(agent) ? FOUNDER_HEADING : LESSONS_HEADING;
    const bytes = items.length ? new TextEncoder().encode(head + lines).length : 0;
    return { agent, items, bytes, cap: 3072 };
  }

  /** Each agent's review level and record per kind of work it has done (the founder's first verdict on each task, newest last); a seed
   *  puts them here. */
  readonly reviews = new Map<string, Map<ReviewWork, Rung>>();
  /** The exact commands the founder always allows each agent (access.list): the architect has two. */
  readonly allowRules = new Map<string, { rule: string; at: string }[]>([
    [
      'handline.architect',
      [
        { rule: 'Bash(pnpm --dir apps/web test)', at: '2026-11-20T09:00:00Z' },
        { rule: 'Bash(go test ./internal/reads/)', at: '2026-11-24T15:30:00Z' },
      ],
    ],
  ]);

  /** agent.review: per kind of work, its level, the last ten of its record, and one step less looked at after ten first-look OKs in a row
   *  (never for contract). */
  reviewOf(agent: string) {
    const ladder = REVIEW_WORKS.flatMap((work) => {
      const r = this.reviews.get(agent)?.get(work);
      if (!r) return [];
      const record = r.record.slice(-10);
      const down = REVIEW_LEVELS[REVIEW_LEVELS.indexOf(r.level) + 1] ?? null;
      const earned = work !== 'contract' && record.length === 10 && record.every((x) => x.ok_first);
      return [{ work, level: r.level, record, looks: r.record.length, suggested: earned ? down : null }];
    });
    return { agent, ladder, lessons: this.lessonsOf(agent).items.length };
  }

  /** agents.list's summary: the least looked-at level among the kinds the agent has done, and that kind (null at every). */
  reviewSummary(agent: string): { level: ReviewLevel; work: ReviewWork | null } {
    let least: { level: ReviewLevel; work: ReviewWork | null } = { level: 'every', work: null };
    for (const [work, r] of this.reviews.get(agent) ?? [])
      if (REVIEW_LEVELS.indexOf(r.level) > REVIEW_LEVELS.indexOf(least.level)) least = { level: r.level, work };
    return least;
  }

  /** agent.review_set: a new level at a kind starts a new record there, and its offer leaves. */
  setReview(agent: string, work: ReviewWork, level: ReviewLevel) {
    const ladder = this.reviews.get(agent) ?? new Map<ReviewWork, Rung>();
    ladder.set(work, { level, record: [] });
    this.reviews.set(agent, ladder);
    this.dismissWarning(`review:${agent}:${work}`);
  }

  /** lesson.remove: false when no live lesson has that id. */
  removeLesson(id: number): boolean {
    for (const [agent, items] of this.lessons) {
      const gone = items.find((l) => l.id === id);
      if (gone)
        return (
          this.lessons.set(
            agent,
            items.filter((l) => l !== gone),
          ),
          this.removedLessons.push(gone),
          true
        );
    }
    return false;
  }

  create(p: Record<string, unknown>, person: string, agent = false): number {
    const owner = String(p.owner ?? '');
    if (owner !== 'human' && !this.isAgent(owner)) throw new TaskFailure('bad_request', `no agent ${owner}`);
    if (typeof p.title !== 'string' || !p.title.trim()) throw new TaskFailure('bad_request', 'a task needs a title');
    if (p.draft !== undefined && typeof p.draft !== 'boolean') throw invalidParams('draft is true or false', 'draft');
    if (p.draft === true && owner === 'human')
      throw new TaskFailure(
        'refused',
        "only an agent's task can be a draft: a draft waits for its assignee's plan and for someone to queue it",
      );
    this.images(String(p.body ?? ''), '', 'body', agent);
    const id = this.nextId++;
    this.seed({
      id,
      from: person,
      to: owner,
      title: p.title,
      body: String(p.body ?? ''),
      state: p.draft === true ? 'draft' : 'queued',
      time_box: String(p.time_box ?? '2h'),
      clock: { length_s: 7200, default: p.time_box === undefined, start: null, end: null, overdue_since: null, told: 0, left_s: null },
      priority: (p.priority as 1 | 2 | 3 | undefined) ?? undefined,
      estimate: typeof p.estimate === 'number' ? p.estimate : null,
      parent: typeof p.parent === 'number' ? p.parent : null,
      blockers: typeof p.blocked_by === 'number' ? [{ task: this.find(p.blocked_by).id, reason: '', at: this.now() }] : [],
    });
    this.find(id).history![0]!.by = person;
    this.emit(this.view(this.find(id)));
    return id;
  }

  edit(p: Record<string, unknown>, agent = false): void {
    const t = this.find(p.task);
    if (t.state === 'discarded' && Object.keys(p).some((k) => !['task', 'body', 'request_id'].includes(k)))
      throw new TaskFailure('refused', `task ${t.id} is closed`);
    if (p.evidence !== undefined && agent)
      throw new TaskFailure('refused', `only the founder can change the result of a done task; add a comment to ${t.id} instead`);
    if (p.evidence !== undefined && String(p.evidence).trim() === '')
      throw new TaskFailure('refused', `a done task keeps a result - an empty one would reopen ${t.id}`);
    if (p.evidence !== undefined && t.state !== 'done')
      throw new TaskFailure('refused', `task ${t.id} is not done - it has no result to edit yet`);
    if (p.owner !== undefined && !this.isAgent(String(p.owner))) throw new TaskFailure('bad_request', `no agent ${String(p.owner)}`);
    if (p.body !== undefined) this.images(String(p.body), t.body, 'body', agent);
    const waitOn = typeof p.blocked_by === 'number' ? this.find(p.blocked_by).id : null;
    const parent = typeof p.parent === 'number' ? this.find(p.parent).id : p.no_parent === true ? null : undefined;
    if (parent !== undefined) this.reparent(t, parent);
    this.change(t, {
      ...(parent !== undefined ? { parent } : {}),
      ...(waitOn !== null && !t.blockers.some((b) => b.task === waitOn)
        ? { blockers: [...t.blockers, { task: waitOn, reason: '', at: this.now() }] }
        : {}),
      ...(p.owner !== undefined ? { to: String(p.owner) } : {}),
      ...(p.owner !== undefined && this.openerAsked(t) ? { turns: [], acting: String(p.owner) } : {}), // its opener's question goes with the old assignee
      ...fieldsOf(p),
    });
  }

  /** As the daemon: the old parent stops waiting on t, an open new one waits on an open t. */
  private reparent(t: Row, parent: number | null): void {
    if (parent === t.parent) throw new TaskFailure('refused', `task T${t.id} already sits there - nothing to move`);
    if (parent !== null && this.below(t.id, this.find(parent)))
      throw new TaskFailure('refused', `task T${t.id} cannot go under T${parent}: it is that task or sits above it`);
    const open = (r: Row) => r.state !== 'done' && r.state !== 'discarded';
    const p = parent === null ? undefined : this.find(parent);
    if (p?.state === 'discarded') throw new TaskFailure('refused', `task T${p.id} is discarded - a task cannot go under it`);
    const old = t.parent === null ? undefined : this.find(t.parent);
    if (old) this.change(old, { blockers: old.blockers.filter((b) => b.task !== t.id) });
    if (p && open(p) && open(t) && !p.blockers.some((b) => b.task === t.id))
      this.change(p, { blockers: [...p.blockers, { task: t.id, reason: '', at: this.now() }] });
  }

  /** Withdraw takes back a task nobody has started. */
  withdraw(p: Record<string, unknown>): void {
    const t = this.find(p.task);
    if (t.state === 'discarded') throw new TaskFailure('refused', `task ${t.id} is already discarded`);
    if (t.state !== 'queued' && t.state !== 'draft' && t.state !== 'pending')
      throw new TaskFailure('refused', `task ${t.id} has started; discard it instead`);
    this.change(t, { state: 'discarded', done: this.now() });
  }
}

// The task methods the fake serves; the rest (handoff) are refused as `unsupported`.
const BASIC = new Set([
  'tasks.list',
  'task.get',
  'task.comment.recipients',
  'task.seen',
  'requests.pending',
  'warning.dismiss',
  'lessons.list',
  'lesson.remove',
  'access.list',
  'access.rule_remove',
  'agent.review',
  'agent.review_set',
  'task.history',
  'task.time',
  'task.changes',
  'task.pr',
  'task.new',
  'task.edit',
  'task.withdraw',
  'task.discard',
  'task.plan',
  'task.plan_ok',
  'task.plan_ask',
  'task.core_ask',
  'task.ok',
  'task.redo',
]);
const VERBS = new Set([
  'task.done',
  'task.reopen',
  'task.queue',
  'task.draft',
  'task.release',
  'task.merge',
  'task.comment.add',
  'task.comment.edit',
  'task.comment.delete',
  'task.turn.pop',
]);
