// The fake daemon's task methods: which ones it serves and how one call is answered by the store.
import { invalidParams, refused, storeFailure } from '#src/errors.ts';
import { discardAll } from '#src/discardbatch.ts';
import { timeOf } from '#src/history.ts';
import { TaskFailure, type FakeTasks } from '#src/tasks.ts';

/** Answers one task method, turning the store's refusals into the daemon's errors. */
export function taskCall(
  tasks: FakeTasks,
  method: string,
  p: Record<string, unknown>,
  person: string,
  agent: boolean,
  script: { accepted(said: string, extra?: Record<string, unknown>): unknown },
  ask: (task: number, to: string, text: string) => number = () => {
    throw refused('unsupported', 'the fake daemon does not serve task.ask');
  },
): unknown {
  const ok = (said: string, extra?: Record<string, unknown>) => script.accepted(said, extra);
  tasks.by = person;
  try {
    switch (method) {
      case 'task.history':
        return tasks.history(p);
      case 'task.time':
        return timeOf(p.task, tasks);
      case 'tasks.list': {
        const list = tasks.list(p);
        // The texts a turn's ask replaced are in task.get only.
        const lean = list.items.map((t) => ({ ...t, turns: t.turns?.map(({ earlier: _, ...x }) => x) }));
        return { ...list, items: agent ? lean.map((t) => ({ ...t, unread: 0, first_unread: null })) : lean };
      }
      case 'task.get': {
        const t = tasks.get(p, agent);
        return agent ? { ...t, unread: 0, first_unread: null } : t;
      }
      case 'task.seen':
        return (tasks.seen(p), ok('Marked seen.'));
      case 'requests.pending':
        return tasks.pending();
      case 'task.comment.list':
        return tasks.comments(p, agent);
      case 'task.comment.recipients':
        return tasks.recipients(p, person);
      case 'task.new':
        return ok('Task asked.', { task: tasks.create(p, person, agent) });
      case 'task.edit':
        return (tasks.edit(p, agent), ok('Task changed.'));
      case 'task.withdraw':
        return (tasks.withdraw(p), ok('Task withdrawn.'));
      case 'task.ask': {
        if (!Number.isInteger(p.task)) throw invalidParams('task is a task number', 'task');
        if (typeof p.to !== 'string' || !p.to) throw invalidParams("to is an agent's name", 'to');
        if (typeof p.text !== 'string' || !p.text.trim()) throw invalidParams('text is the question, not empty', 'text');
        return ok('Asked.', { request: ask(tasks.find(p.task).id, p.to, p.text.trim()) });
      }
      case 'task.done':
        return ok(tasks.done(p));
      case 'task.reopen':
        return (tasks.reopen(p, person), ok('Task reopened.'));
      case 'task.queue':
        return (tasks.move(p, ['draft'], 'queued'), ok('Queued.'));
      case 'task.draft':
        return (tasks.move(p, ['queued', 'doing'], 'draft'), ok('Back to draft.'));
      case 'task.discard':
        return (Array.isArray(p.tasks) ? discardAll(tasks, p) : tasks.discard(p), ok('Task discarded.'));
      case 'task.release':
        return (tasks.release(p), ok('Task released.'));
      case 'task.merge':
        return (tasks.merge(p), ok('Tasks folded.'));
      case 'task.comment.add':
        if (typeof p.done === 'string' && p.done.trim() && (p.turn === undefined || p.turn === 'human'))
          throw invalidParams('done goes with a turn to an agent: a review whose pass closes the task', 'done');
        return ok('Comment added.', { memo: tasks.memo(p, 'add', person, agent) });
      case 'task.comment.edit':
        return (tasks.memo(p, 'edit', person, agent), ok('Comment changed.'));
      case 'task.comment.delete':
        return (tasks.memo(p, 'delete', person), ok('Comment deleted.'));
      case 'task.turn.pop':
        return (tasks.turnPop(p), ok('Turn given back.'));
      case 'task.comment.send_now':
        return (tasks.sendNow(p), ok('Sent.'));
      default:
        throw refused('unsupported', `the fake daemon does not serve ${method}`);
    }
  } catch (e) {
    if (!(e instanceof TaskFailure)) throw e;
    throw storeFailure(e);
  }
}
