// Every write method of the contract, once, with its kind and what it does to the cache (docs/turborepo-layout.md §8.5).
// A method in none of the four lists fails the guard test, so a new write is decided by a person before it ships.
import { navOf, taskAfter, type PageSettings, type TaskEdit, type TaskVerb } from '@handline/app-core';
import type {
  Account,
  InboxItem,
  Memo,
  NavPrefsChanges,
  Prefs,
  Proposal,
  Schedule,
  Settings,
  StandingAnswer,
  Task,
} from '@handline/client';
import { edit, provisionalId, upsert, type Ctx } from '#src/cache.ts';
import { accountAfter, type AccountChange } from '#src/account-after.ts';
import { accountRemove } from '#src/account-remove.ts';
import { chatWrites } from '#src/chat-writes.ts';
import { classify } from '#src/errors.ts';
import { APP_WRITES, type WriteKind } from '@handline/client';
import { keyOf, qk } from '#src/reads.ts';
import type { Patch, WriteSpec } from '#src/write-spec.ts';

const as = <T>(p: never) => p as T;

type List<K extends string, T> = Record<K, T[]>;
const listOf =
  <K extends string, T>(key: K, f: (items: T[]) => T[]) =>
  (l: List<K, T>) => ({ ...l, [key]: f(l[key]) });

// ---- tasks ----

const taskKeys = (d: string, p: never) => {
  const { task, tasks } = as<{ task?: number; tasks?: number[] }>(p);
  return [qk.tasks(d), ...[...(task === undefined ? [] : [task]), ...(tasks ?? [])].flatMap((t) => [qk.task(d, t), qk.taskHistory(d, t)])];
};
const changeTasks = (c: Ctx, ids: number[], f: (t: Task) => Task | undefined) => {
  const one = (t: Task) => (ids.includes(t.id) ? (f(t) ?? t) : t);
  edit<List<'items', Task>>(
    c.qc,
    qk.tasks(c.d),
    listOf('items', (l) => l.map(one)),
  );
  for (const id of ids) edit<Task>(c.qc, qk.task(c.d, id), one);
};
const verb = (name: TaskVerb): Patch => ({
  touches: taskKeys,
  patch: (c, p) => {
    const { task, tasks, owner, title, time_box, priority } = as<{ task?: number; tasks?: number[] } & TaskEdit>(p);
    changeTasks(c, tasks ?? [task!], (t) => taskAfter(name, t, c.now, { owner, title, time_box, priority }));
  },
});

/** A provisional row's own id is replaced by the daemon's number, or the row goes if the daemon's own event already added the real one. */
function renumber<T extends { id: number }>(rows: T[], provisional: (r: T) => boolean, id: number): T[] {
  const real = rows.some((r) => r.id === id);
  return rows.flatMap((r) => (provisional(r) ? (real ? [] : [{ ...r, id }]) : [r]));
}

const newTask = (c: Ctx, p: never): Task => {
  const v = as<{ owner: string; title: string; time_box?: string; parent?: number }>(p);
  return {
    id: provisionalId(),
    from: c.me,
    to: v.owner,
    title: v.title,
    state: 'pending',
    time_box: v.time_box ?? null,
    parent: v.parent ?? null,
    created: c.now,
    delivered: null,
    done: null,
    hold_reason: null,
    block_reason: null,
    evidence: null,
  };
};

// A task's memos live in the memo list and in the task's detail; a memo write changes both, and adds a row to its history.
const memoLists = (d: string, task: number) => [qk.memos(d, task), qk.task(d, task)];
const memoKeys = (d: string, p: never) => {
  const { task } = as<{ task: number }>(p);
  return [...memoLists(d, task), qk.taskHistory(d, task)];
};
const changeMemos = (c: Ctx, task: number, f: (l: Memo[]) => Memo[]) => {
  for (const key of memoLists(c.d, task)) edit<List<'memos', Memo>>(c.qc, key, listOf('memos', f));
};

const tasks: Record<string, Patch> = {
  'task.new': {
    touches: (d) => [qk.tasks(d)],
    patch: (c, p) =>
      edit<List<'items', Task>>(
        c.qc,
        qk.tasks(c.d),
        listOf('items', (l) => [newTask(c, p), ...l]),
      ),
    reconcile: (c, _p, r) => {
      const id = (r as { task: number }).task;
      edit<List<'items', Task>>(
        c.qc,
        qk.tasks(c.d),
        listOf('items', (l) => renumber(l, (t) => t.id < 0, id)),
      );
    },
  },
  'task.done': verb('done'),
  'task.reopen': verb('reopen'),
  'task.queue': verb('queue'),
  'task.draft': verb('draft'),
  'task.release': verb('release'),
  'task.withdraw': verb('withdraw'),
  'task.cancel': verb('cancel'),
  'task.edit': verb('edit'),
  'task.merge': {
    touches: (d) => [qk.tasks(d)],
    patch: (c, p) => {
      const gone = as<{ tasks: number[] }>(p).tasks;
      edit<List<'items', Task>>(
        c.qc,
        qk.tasks(c.d),
        listOf('items', (l) => l.filter((t) => !gone.includes(t.id))),
      );
    },
  },
  'task.comment.add': {
    touches: memoKeys,
    patch: (c, p) => {
      const { task, text, reply } = as<{ task: number; text: string; reply?: number }>(p);
      const memo: Memo = {
        id: provisionalId(),
        task,
        author: c.author ?? c.me,
        at: c.now,
        text,
        ...(reply === undefined ? {} : { reply_to: reply }),
      };
      changeMemos(c, task, (l) => [...l, memo]);
    },
    reconcile: (c, p, r) => changeMemos(c, as<{ task: number }>(p).task, (l) => renumber(l, (m) => m.id < 0, (r as { memo: number }).memo)),
  },
  'task.comment.edit': {
    touches: memoKeys,
    patch: (c, p) => {
      const { task, memo, text } = as<{ task: number; memo: number; text: string }>(p);
      changeMemos(c, task, (l) => l.map((m) => (m.id === memo ? { ...m, text } : m)));
    },
  },
  // The question is a message to the holder linked to the task; the task page reads it again for the event it leaves.
  'task.ask': {
    touches: (d, p) => [qk.task(d, as<{ task: number }>(p).task)],
  },
  'task.comment.delete': {
    touches: memoKeys,
    patch: (c, p) => {
      const { task, memo } = as<{ task: number; memo: number }>(p);
      changeMemos(c, task, (l) => l.filter((m) => m.id !== memo));
    },
  },
  // The tasks topic's event brings the task again; the params name no task, so the comment lists are read again here.
  'task.comment.send_now': { touches: (d) => [qk.resource(d, 'task.comment.list')] },
  // The event brings every place the image is named; the dry run only counts them.
  'task.view.save': { touches: (d) => [qk.taskViews(d)] },
  'task.view.delete': { touches: (d) => [qk.taskViews(d)] },
};

// ---- inbox, settings, accounts, schedules, standing answers ----

const inboxKeys = (d: string) => [qk.inboxes(d)];
const markItems = (c: Ctx, ids: number[], patch: Partial<InboxItem>) =>
  edit<List<'items', InboxItem>>(
    c.qc,
    qk.inboxes(c.d),
    listOf('items', (l) => l.map((i) => (ids.includes(i.id) ? { ...i, ...patch } : i))),
  );

const settings: Record<string, Patch> = {
  'inbox.mark_read': {
    quiet: true,
    touches: inboxKeys,
    patch: (c, p) => markItems(c, as<{ ids: number[] }>(p).ids, { read: true }),
  },
  'inbox.reply': {
    touches: inboxKeys,
    patch: (c, p) => markItems(c, [as<{ id: number }>(p).id], { read: true }),
  },
  'agent.set': {
    touches: (d, p) => [qk.agent(d, as<{ agent: string }>(p).agent)],
    patch: (c, p) => {
      const { agent, values } = as<{ agent: string; values: object }>(p);
      edit<{ settings?: object }>(c.qc, qk.agent(c.d, agent), (a) => ({ ...a, settings: { ...a.settings, ...values } }));
    },
    // The strip's numbers (the context maximum among them) come from the settings but ride no event of their own: read them again.
    reconcile: (c, p) => void c.qc.invalidateQueries({ queryKey: keyOf(c.d, 'agent.strip', { agent: as<{ agent: string }>(p).agent }) }),
  },
  'settings.set': {
    // network_addresses moves the doors: the addresses and the listeners are read again.
    touches: (d) => [qk.resource(d, 'settings.get'), qk.resource(d, 'connection.addresses'), qk.resource(d, 'connection.get')],
    // A per-agent override is not the shared value, so only the shared one is shown early; null is the default, which only the daemon knows.
    patch: (c, p) => {
      const { agent, changes } = as<{ agent?: string; changes: PageSettings }>(p);
      const set = Object.fromEntries(Object.entries(changes).filter(([, v]) => v !== null));
      if (!agent) edit<Settings>(c.qc, qk.resource(c.d, 'settings.get'), (s) => ({ ...s, values: { ...s.values, ...set } }));
    },
  },
  'prefs.set': {
    touches: (d) => [qk.prefs(d)],
    patch: (c, p) => {
      const { nav } = as<{ changes: { nav: NavPrefsChanges } }>(p).changes;
      edit<Prefs>(c.qc, qk.prefs(c.d), (s) => {
        const { later, ...rest } = nav;
        const merged = { ...navOf(s).later };
        for (const [k, v] of Object.entries(later ?? {}))
          if (v === null) delete merged[k];
          else merged[k] = v;
        return { ...s, nav: { ...navOf(s), ...rest, ...(later && { later: merged }) } };
      });
    },
  },
  'trust.remove': {
    touches: (d) => [qk.resource(d, 'trust.list')],
    patch: (c, p) =>
      edit<List<'roots', string>>(
        c.qc,
        qk.resource(c.d, 'trust.list'),
        listOf('roots', (l) => l.filter((r) => r !== as<{ root: string }>(p).root)),
      ),
  },
  'account.set': {
    touches: (d) => [qk.accounts(d)],
    patch: (c, p) => {
      const { account } = as<{ account: string }>(p);
      edit<List<'accounts', Account>>(
        c.qc,
        qk.accounts(c.d),
        listOf('accounts', (l) => l.map((a) => (a.name === account ? accountAfter(a, as<AccountChange>(p)) : a))),
      );
    },
  },
  // Wake on a stalled account row, as on a stalled agent's: the row shows it usable at once.
  'account.wake': {
    touches: (d) => [qk.accounts(d)],
    patch: (c, p) => {
      const { account } = as<{ account: string }>(p);
      edit<List<'accounts', Account>>(
        c.qc,
        qk.accounts(c.d),
        listOf('accounts', (l) => l.map((a) => (a.name === account ? { ...a, stalled: null } : a))),
      );
    },
  },
};

const scheduleKeys = (d: string) => [qk.resource(d, 'schedules.list')];
const editSchedules = (c: Ctx, f: (l: Schedule[]) => Schedule[]) =>
  edit<List<'schedules', Schedule>>(c.qc, qk.resource(c.d, 'schedules.list'), listOf('schedules', f));
const standing = (p: never): StandingAnswer => {
  const { folder, vendor, title, label } = as<{ folder?: string; vendor?: string; title?: string; label?: string }>(p);
  return folder
    ? { kind: 'trust', key: [folder], text: folder }
    : { kind: 'dialog', key: [vendor ?? '', title ?? '', label ?? ''], text: label ?? '' };
};

const schedules: Record<string, Patch> = {
  'schedule.add': {
    touches: scheduleKeys,
    patch: (c, p) => {
      const { target, cron, message } = as<{ target: string; cron: string; message: string }>(p);
      editSchedules(c, (l) => [...l, { id: provisionalId(), target, cron, message, next_run: null, last_run: null, set_by: c.me }]);
    },
    reconcile: (c, _p, r) => editSchedules(c, (l) => renumber(l, (s) => s.id < 0, (r as { schedule: number }).schedule)),
  },
  'schedule.edit': {
    touches: scheduleKeys,
    patch: (c, p) => {
      const { schedule, target, cron, message } = as<Partial<Schedule> & { schedule: number }>(p);
      const set = { ...(target ? { target } : {}), ...(cron ? { cron } : {}), ...(message ? { message } : {}) };
      editSchedules(c, (l) => l.map((s) => (s.id === schedule ? { ...s, ...set } : s)));
    },
  },
  'schedule.remove': {
    touches: scheduleKeys,
    patch: (c, p) => editSchedules(c, (l) => l.filter((s) => s.id !== as<{ schedule: number }>(p).schedule)),
  },
  'answer.add': {
    touches: (d) => [qk.resource(d, 'answers.list')],
    patch: (c, p) =>
      edit<List<'answers', StandingAnswer>>(
        c.qc,
        qk.resource(c.d, 'answers.list'),
        listOf('answers', (l) => [...l, standing(p)]),
      ),
  },
  'answer.remove': {
    touches: (d) => [qk.resource(d, 'answers.list')],
    patch: (c, p) => {
      const { kind, key } = as<{ kind: string; key: string[] }>(p);
      edit<List<'answers', StandingAnswer>>(
        c.qc,
        qk.resource(c.d, 'answers.list'),
        listOf('answers', (l) => l.filter((a) => !(a.kind === kind && a.key.join('\0') === key.join('\0')))),
      );
    },
  },
};

const askedDirty = (e: unknown) => {
  const f = classify(e);
  return f.kind === 'refused' && f.gate === 'dirty';
};

export const patches: Record<string, Patch> = {
  'account.remove': accountRemove,
  ...chatWrites,
  ...tasks,
  ...settings,
  ...schedules,
  'chat.seen': { quiet: true },
  'task.seen': { quiet: true },
  // A worktree that holds work is refused until the person answers; the screen asks, so no error line is raised.
  'agent.retire': { asks: askedDirty },
  // A prompt the person answers: a stale one is refused and the card shows the prompt that is open now.
  'prompt.answer': {
    fail: (c, p, e) => {
      const current = (e as { code?: string; data?: { current?: unknown } }).data?.current;
      if ((e as { code?: string }).code !== 'stale_prompt') return false;
      const agent = as<{ agent: string }>(p).agent;
      c.qc.setQueryData(qk.prompts(c.d, agent), { prompts: current ? [current] : [] });
      edit<{ prompt: unknown }>(c.qc, qk.agent(c.d, agent), (d) => ({ ...d, prompt: current ?? null }));
      return true;
    },
  },
  // A decide on a proposal that changed or settled meanwhile: the refusal carries the proposal as it is now, and the card shows that.
  'proposal.decide': {
    fail: (c, _p, e) => {
      void c.qc.invalidateQueries({ queryKey: qk.hrs(c.d) });
      const proposal = (e as { data?: { proposal?: Proposal } }).data?.proposal;
      if (!proposal) return false;
      edit<{ proposals: Proposal[] }>(c.qc, qk.resource(c.d, 'proposal.list'), (l) => ({
        ...l,
        proposals: upsert(l.proposals, proposal, (x) => x.id === proposal.id),
      }));
      return true;
    },
  },
};

/** Every write the app makes: the spec's kind and label with the patch registered under the method's name. */
export const writes: Record<string, WriteSpec> = Object.fromEntries(
  Object.entries(APP_WRITES as Record<string, { kind: WriteKind; label: string } | false>).flatMap(([m, facts]) =>
    facts ? [[m, { ...facts, ...patches[m] }]] : [],
  ),
);
