// The verbs of the daemon's task plan (D3), served only when the fake is asked to.
import { TaskComments } from '#src/taskcomments.ts';
import { TaskFailure } from '#src/taskstore.ts';

export class TaskVerbs extends TaskComments {
  /** A done closes the task; a review is a task.handoff of the task itself. */
  done(p: Record<string, unknown>): string {
    const t = this.find(p.task);
    if (t.state !== 'doing' && t.state !== 'queued') throw new TaskFailure('refused', `task ${t.id} is ${t.state}, not open`);
    const evidence = String(p.evidence ?? '') || 'Done';
    this.change(t, { state: 'done', done: this.now(), evidence });
    this.host.inbox.closeAsks(t.id);
    return `task T${t.id} done`;
  }
  /** The founder goes on top with the ask; already on top, a new ask rewrites it (edited, old text in earlier) and the same text changes nothing. */
  turnPush(p: Record<string, unknown>, agent: string): void {
    const t = this.find(p.task);
    if (p.to !== 'human') throw new TaskFailure('refused', 'the fake daemon pushes turns to human only');
    const ask = String(p.ask ?? '').trim();
    const top = t.turns?.at(-1);
    if (top?.agent === 'human') {
      if (top.ask === ask) return;
      const turn = { ...top, ask, edited: this.now(), earlier: [...(top.earlier ?? []), { ask: top.ask, at: top.edited ?? top.since }] };
      this.change(t, { turns: [...t.turns!.slice(0, -1), turn] });
      return;
    }
    const turn = { agent: 'human', ask, by: agent || t.to, since: this.now(), on_pass: null, evidence: null, edited: null, earlier: [] };
    const log = [...(t.turnLog ?? []), { at: turn.since, who: `${turn.by} → human`, text: ask, turn: 'push' as const }];
    this.change(t, { turns: [...(t.turns ?? []), turn], acting: 'human', turnLog: log });
  }
  /** The founder gives their turn back to whoever pushed it. */
  turnPop(p: Record<string, unknown>): void {
    const t = this.find(p.task);
    const top = t.turns?.at(-1);
    if (top?.agent !== 'human') throw new TaskFailure('refused', `task ${t.id} is not the founder's turn`);
    const log = [...(t.turnLog ?? []), { at: this.now(), who: `human → ${top.by}`, text: '', turn: 'pop' as const }];
    this.change(t, { turns: t.turns!.slice(0, -1), acting: top.by, turnLog: log });
  }
  reopen(p: Record<string, unknown>, person: string): void {
    const t = this.find(p.task);
    if ((t.state !== 'done' && t.state !== 'discarded') || t.from !== person)
      throw new TaskFailure('refused', 'only a done or discarded task that you asked for can be reopened');
    this.change(t, { state: t.state === 'done' ? 'queued' : 'draft', done: null });
  }
  /** Queue or draft a list: the whole call is refused if any of them is in the wrong state. */
  move(p: Record<string, unknown>, from: readonly string[], to: 'draft' | 'queued'): void {
    const rows = ((p.tasks as number[]) ?? []).map((id) => this.find(id));
    const bad = rows.find((t) => !from.includes(t.state));
    if (bad) throw new TaskFailure('refused', `task ${bad.id} is ${bad.state}, not ${from.join(' or ')}`);
    for (const t of rows) this.change(t, { state: to });
  }
  discard(p: Record<string, unknown>): void {
    const t = this.find(p.task);
    if (t.state === 'done' || t.state === 'discarded') throw new TaskFailure('refused', `task ${t.id} is ${t.state}`);
    this.change(t, { state: 'discarded', done: this.now(), hold_reason: String(p.reason ?? '').trim() || 'Discarded' });
    this.host.inbox.closeAsks(t.id);
  }
  /** Take a doing task back to queued; the daemon takes no target and no reason. */
  release(p: Record<string, unknown>): void {
    if ('to' in p) throw new TaskFailure('bad_request', 'task.release takes no to');
    const t = this.find(p.task);
    if (t.state !== 'doing') throw new TaskFailure('refused', `task ${t.id} is ${t.state}, not doing`);
    this.change(t, { state: 'queued' });
  }
  merge(p: Record<string, unknown>): void {
    const into = this.find(p.into);
    const rows = ((p.tasks as number[]) ?? []).map((id) => this.find(id));
    if (rows.some((t) => t.id === into.id)) throw new TaskFailure('refused', 'a task cannot be folded into itself');
    for (const t of rows) this.change(t, { state: 'discarded', done: this.now(), hold_reason: `folded into ${into.id}` });
  }
}
