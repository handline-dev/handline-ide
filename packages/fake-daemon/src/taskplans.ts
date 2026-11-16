// The plan writes: the assignee keeps a plan (task.plan), the founder marks it OK (task.plan_ok) and asks for a plan or the core logic
// (task.plan_ask, task.core_ask). The fake has no agents to wait behind: an ask is sent at once.
import { invalidParams } from '#src/errors.ts';
import { planState } from '#src/planview.ts';
import { TaskFailure, type Row } from '#src/taskstore.ts';
import { TaskVerbs } from '#src/taskverbs.ts';

export class TaskPlans extends TaskVerbs {
  private openAgentTask(id: unknown): Row {
    if (!Number.isInteger(id)) throw invalidParams('task is a task number', 'task');
    const t = this.find(id);
    if (t.state === 'done' || t.state === 'discarded') throw new TaskFailure('refused', `T${t.id} has ended`);
    if (!this.isAgent(t.to)) throw new TaskFailure('refused', `T${t.id} is not an agent's task: only its assignee writes its plan`);
    return t;
  }

  writePlan(p: Record<string, unknown>, person: string): number {
    if (typeof p.text !== 'string' || !p.text.trim()) throw invalidParams('text is text and not empty', 'text');
    const t = this.openAgentTask(p.task);
    if (person !== t.to) throw new TaskFailure('refused', `T${t.id}'s plan is written by its assignee, ${t.to}`);
    const versions = t.plan?.versions ?? [];
    const text = p.text.trim();
    if (versions.at(-1)?.text === text) throw new TaskFailure('refused', `T${t.id}'s plan already reads so`);
    this.change(t, { plan: { versions: [...versions, { text, by: person, at: this.now() }], ok: null }, planAsk: null });
    return versions.length + 1;
  }

  markPlan(p: Record<string, unknown>, person: string): void {
    if (p.ok !== undefined && typeof p.ok !== 'boolean') throw invalidParams('ok is true or false', 'ok');
    const ok = p.ok !== false;
    if (!Number.isInteger(p.task)) throw invalidParams('task is a task number', 'task');
    const t = this.find(p.task);
    if (t.state === 'done' || t.state === 'discarded') throw new TaskFailure('refused', `T${t.id} has ended`);
    if (!t.plan) throw new TaskFailure('refused', `T${t.id} has no plan yet`);
    if (p.version !== undefined && !Number.isInteger(p.version)) throw invalidParams('version is a plan version number', 'version');
    if (p.version !== undefined && p.version !== t.plan.versions.length)
      throw new TaskFailure(
        'refused',
        `T${t.id}'s plan is at version ${t.plan.versions.length} now, not ${String(p.version)}: read it again`,
      );
    if (!!t.plan.ok === ok) throw new TaskFailure('refused', `T${t.id}'s plan is already ${ok ? 'OK' : 'not OK'}`);
    this.change(t, { plan: { ...t.plan, ok: ok ? { by: person, at: this.now() } : null } });
  }

  askPlan(p: Record<string, unknown>, person: string): void {
    const t = this.openAgentTask(p.task);
    const state = planState(t);
    if (state === 'asked') throw new TaskFailure('refused', `${t.to} is already asked for T${t.id}'s plan`);
    this.change(t, { planAsk: { at: this.now(), by: person, to: t.to, sent_at: this.now() } });
  }

  askCore(p: Record<string, unknown>, person: string): void {
    if (p.note !== undefined && p.note !== null && typeof p.note !== 'string') throw invalidParams('note is text', 'note');
    const t = this.openAgentTask(p.task);
    if (t.coreAsk && !t.coreAsk.sent_at) throw new TaskFailure('refused', `${t.to} is already asked for T${t.id}'s core logic`);
    const note = typeof p.note === 'string' && p.note.trim() ? p.note.trim() : null;
    this.change(t, { coreAsk: { at: this.now(), by: person, to: t.to, sent_at: this.now(), note } });
  }
}
