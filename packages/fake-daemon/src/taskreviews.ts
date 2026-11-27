// The founder's verdict on a story's sub-task that is Ready (TaskDetail.review): OK, merge hands it to the land_by agent it waited for, Redo
// sends it back to its assignee with a note. The fake serves no agent's handoff, so a seed makes a task Ready.
import { invalidParams } from '#src/errors.ts';
import { TaskFailure, type Row } from '#src/taskstore.ts';
import { TaskPlans } from '#src/taskplans.ts';

export class TaskReviews extends TaskPlans {
  private readyTask(p: Record<string, unknown>): Row & { review: NonNullable<Row['review']> } {
    if (!Number.isInteger(p.task)) throw invalidParams('task is a task number', 'task');
    const t = this.find(p.task);
    if (!t.review?.ready || t.state === 'done' || t.state === 'discarded')
      throw new TaskFailure('refused', `T${t.id} is not Ready for you: only a story's sub-task its assignee finished waits for OK or Redo`);
    return t as Row & { review: NonNullable<Row['review']> };
  }

  ok(p: Record<string, unknown>): string {
    const t = this.readyTask(p);
    const to = t.review.land_by;
    const handoffLog = [...(t.handoffLog ?? []), { at: this.now(), who: `human → ${to}`, text: 'OK, merge' }];
    this.change(t, { turns: t.turns?.slice(0, -1), acting: to, to, handoffLog, review: { ...t.review, ready: false, ok: true } });
    return `T${t.id} is OK: handed to ${to} to merge`;
  }

  redo(p: Record<string, unknown>, person: string): number {
    if (typeof p.note !== 'string' || !p.note.trim()) throw invalidParams('a redo says what to change in note', 'note');
    const t = this.readyTask(p);
    this.change(t, { review: { ...t.review, ready: false, redo: t.review.redo + 1 } });
    return this.comment({ task: t.id, text: p.note }, 'add', person); // no longer Ready, the founder's comment passes their turn back
  }
}
