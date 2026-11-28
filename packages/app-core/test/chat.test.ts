import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { ChatMessage, TranscriptItem } from '@handline/client';
import { setup } from '@handline/fake-daemon/helpers';
import {
  Store,
  isTaskDelivery,
  noticeOf,
  tickLevel,
  handoffLine,
  sideOf,
  timeline,
  foldBash,
  foldTurns,
  unblockedStart,
  maybeRanNote,
  restartingLine,
  turnMinutes,
} from '#src/index.ts';

const A = 'handline.architect';
const msg = (over: Partial<ChatMessage>): ChatMessage => ({
  id: 1,
  at: '2026-01-01T00:00:00Z',
  state_at: '2026-01-01T00:00:00Z',
  from: 'founder',
  to: A,
  state: 'delivered',
  landing: 'delivered',
  sending_reason: null,
  body: 'hi',
  delivered_at: null,
  typed_at: null,
  landed_at: null,
  parent: null,
  kind: 'message',
  tasks: [],
  ...over,
});
const item = (over: Partial<TranscriptItem>): TranscriptItem => ({ id: 1, at: '2026-01-01T00:00:00Z', kind: 'reply', text: 'ok', ...over });

test('a message sent during a restart and delivered to the new session reads below its session line', () => {
  const sent = msg({ id: 2, at: '2026-01-01T00:01:00Z', delivered_at: '2026-01-01T00:03:00Z' });
  const before = msg({ id: 1, at: '2026-01-01T00:00:00Z', delivered_at: '2026-01-01T00:00:01Z' });
  const session = item({
    id: 7,
    at: '2026-01-01T00:02:00Z',
    kind: 'session',
    session: { id: 's', started: '2026-01-01T00:02:00Z', handoff: null },
  });
  const out = timeline([sent, before], [session]);
  assert.deepEqual(
    out.map((e) => `${e.kind}${e.id}`),
    ['message1', 'item7', 'message2'],
  );
});

test('a message that wakes an agent reads below the new session line: it is typed after the start, though marked delivered before', () => {
  const wake = msg({ id: 2, at: '2026-01-01T00:01:58Z', delivered_at: '2026-01-01T00:01:58Z', typed_at: '2026-01-01T00:02:01Z' });
  const session = item({
    id: 7,
    at: '2026-01-01T00:02:00Z',
    kind: 'session',
    session: { id: 's', started: '2026-01-01T00:02:00Z', handoff: null },
  });
  assert.deepEqual(
    timeline([wake], [session]).map((e) => `${e.kind}${e.id}`),
    ['item7', 'message2'],
  );
});

test('the timeline puts messages and transcript items in time order, a message first at the same time', () => {
  const out = timeline(
    [msg({ id: 2, at: '2026-01-01T00:02:00Z' }), msg({ id: 1, at: '2026-01-01T00:00:00Z' })],
    [item({ id: 5, at: '2026-01-01T00:01:00Z' }), item({ id: 6, at: '2026-01-01T00:02:00Z' })],
  );
  assert.deepEqual(
    out.map((e) => `${e.kind}${e.id}`),
    ['message1', 'item5', 'message2', 'item6'],
  );
});

test('the message that tells an agent how a proposal was decided is left out of its chat, the card shows it', () => {
  const outcome = msg({ id: 4, body: 'proposal 858cde9b applied: x', proposal: 7 });
  const ids = (scope: 'mine' | 'all') => timeline([outcome, msg({ id: 5, body: 'ok' })], [], scope).map((e) => e.id);
  assert.deepEqual(ids('mine'), [5]);
  assert.deepEqual(ids('all'), [4, 5]);
});

test("a reply with the same words as a message is still shown: the agent answering 'ok' to 'ok'", () => {
  const out = timeline([msg({ id: 3, body: 'ok' })], [item({ id: 8, text: 'ok' })]);
  assert.deepEqual(
    out.map((e) => `${e.kind}${e.id}`),
    ['message3', 'item8'],
  );
});

test('the store knows which chat methods the daemon serves, and subscribes to the chat topic only then', async () => {
  for (const chat of [true, false]) {
    const s = setup({ daemon: { chat } });
    const store = new Store(s.client);
    await store.start();
    assert.equal(store.can('chat.send'), chat);
    assert.equal(store.can('agents.list'), true);
    store.stop();
  }
});

const taskItem = (event: string, extra: object = {}): TranscriptItem =>
  ({
    id: 1,
    at: '2026-10-12T10:00:00Z',
    kind: 'task_event',
    task_event: { event, task: 7, request: null, evidence: null, who: 'human', ...extra },
  }) as TranscriptItem;

const pm = (o: Partial<ChatMessage>) =>
  ({ id: 5, from: 'Handline desktop', to: 'handline.x', landing: 'queued', state: 'queued', ...o }) as ChatMessage;

test("only the person's own words are on their side: the CLI, this device, to whichever agent", () => {
  assert.equal(sideOf(pm({}), A, 'Handline desktop'), 'person');
  assert.equal(sideOf(pm({ from: 'human' }), A, 'Handline desktop'), 'person');
  assert.equal(sideOf(pm({ from: 'handline.y' }), A, 'Handline desktop'), 'other');
  assert.equal(sideOf(pm({ from: 'sys', to: A }), A, 'Handline desktop'), 'other');
  assert.equal(sideOf(pm({ from: 'schedule', to: A }), A, ''), 'other');
});

test('the ticks: none until sent, one sent and not read, two read', () => {
  assert.equal(tickLevel(pm({ id: -1 })), 0);
  assert.equal(tickLevel(pm({})), 0);
  assert.equal(tickLevel(pm({ landing: 'typed' })), 1);
  assert.equal(tickLevel(pm({ landing: 'delivered' })), 2);
  assert.equal(tickLevel(pm({ landing: 'delivered', receipt: { state: 'sent' } as ChatMessage['receipt'] })), 1);
  assert.equal(tickLevel(pm({ landing: 'delivered', receipt: { state: 'confirmed' } as ChatMessage['receipt'] })), 2);
});

test("the task board's words are left out of the agent's own chat and shown in the All view", () => {
  const news = pm({ id: 7, from: 'sys', to: A, body: 'T3492 done by handline.pair: -' }); // local-ref-ok
  const delivery = pm({ id: 8, from: 'sys', to: A, body: '#28387 to handline.pair was written to its session' }); // local-ref-ok
  assert.deepEqual(noticeOf(news), { kind: 'task', task: 3492 });
  assert.deepEqual(noticeOf(delivery), { kind: 'delivery' });
  assert.equal(noticeOf(pm({ from: 'human', body: 'T1 done by me' })), null);
  const kinds = (scope: 'mine' | 'all') => timeline([news, delivery], [], scope).map((e) => `${e.kind}${e.id}`);
  const waits = pm({ id: 9, from: 'sys', to: A, body: 'task T4136 now waits on T4148' }); // local-ref-ok
  assert.deepEqual(
    timeline([news, delivery, waits, pm({ id: 10, from: 'human', to: A })], [], 'mine').map((e) => `${e.kind}${e.id}`),
    ['message10'],
  );
  assert.deepEqual(kinds('mine'), []);
  assert.deepEqual(kinds('all'), ['note7', 'message8']);
  assert.equal(sideOf(delivery, A, 'Handline desktop'), 'other');
});

test('back-to-back restarts are one line saying all that changed; a resume that failed into a fresh start is that fresh start; a restart after a new conversation is its own line', () => {
  const at = '2026-01-01T00:00:00Z';
  const start = (id: number, session: object) =>
    item({ id, at, kind: 'session', session: { id: 's', started: at, handoff: null, ...session } });
  const lines = (x: object, y: object) =>
    timeline([], [start(1, x), start(2, y)]).map((e) => (e as Extract<ReturnType<typeof timeline>[0], { kind: 'item' }>).item.session);
  const model = 'model changed (opus-5-5 -> sonnet-5-5)';
  assert.deepEqual(
    lines({ resumed: true, changed: model }, { resumed: true, changed: '' }).map((s) => [s?.resumed, s?.changed]),
    [[true, model]],
  );
  assert.equal(lines({ resumed: true, changed: model }, { resumed: true, changed: 'team changed' })[0]?.changed, `${model}, team changed`);
  const rules = 'team rules changed (shared/team/a.md, shared/team/b.md)';
  assert.equal(lines({ resumed: true, changed: rules }, { resumed: true, changed: rules })[0]?.changed, rules);
  const failed = lines({ resumed: true }, { resumed: false });
  assert.deepEqual([failed.length, failed[0]?.resumed], [1, false]);
  const after = lines({ resumed: false, cause: 'launch' }, { resumed: true, cause: 'settings_applied', changed: model });
  assert.deepEqual(
    after.map((s) => [s?.resumed, s?.changed]),
    [
      [false, undefined],
      [true, model],
    ],
  );
  assert.equal(lines({ cause: 'launch' }, { cause: 'context_full' }).length, 1);
});

test('an unblocked card is in the chat of the agent the task starts for, though another agent closed the blocker', () => {
  const freed = taskItem('unblocked', { who: 'bob', closed_task: 8 });
  assert.equal(timeline([], [freed], 'mine', 'alice').length, 1);
});

test('handoffLine says why and what for each phase', () => {
  assert.deepEqual(handoffLine({ cause: 'context_full', phase: 'writing' }), { why: 'Context full', what: 'Writing the handoff' });
  assert.equal(handoffLine({ cause: 'context_full', phase: 'starting' }).what, 'Starting a new session');
  assert.equal(handoffLine({ cause: 'odd', phase: 'writing' }).why, 'odd');
});

test('in the all room, what the agent says to another agent is not shown; what it says to the person, and what reaches it, is', () => {
  const ms = [
    msg({ id: 1, from: A, to: 'handline.auditor', body: 'say to auditor' }),
    msg({ id: 2, from: 'handline.auditor', to: A, body: 'say to me' }),
    msg({ id: 3, from: A, to: 'human', body: 'to the person' }),
    msg({ id: 4, from: A, to: 'Handline desktop', body: 'to the device' }),
  ];
  assert.deepEqual(
    timeline(ms, [], 'all', A, 'Handline desktop').map((e) => e.id),
    [2, 3, 4],
  );
  assert.deepEqual(
    timeline(ms, [], 'mine', A, 'Handline desktop').map((e) => e.id),
    [1, 2, 3, 4],
  ); // mine is left as it was
  assert.deepEqual(
    timeline(ms, [], 'all', A).map((e) => e.id),
    [2, 3],
  ); // no `me`: a device name is not known as the person
});

test('a restart is one line: back-to-back session starts collapse and keep the cause and the handoff', () => {
  const at = (n: number) => `2026-01-01T00:00:0${n}Z`;
  const start = (id: number, cause?: string, handoff: string | null = null) =>
    ({
      id,
      at: at(id),
      kind: 'session',
      session: { id: `s${id}`, started: at(id), handoff, ...(cause ? { cause } : {}) },
    }) as unknown as TranscriptItem;
  const said = pm({ id: 9, from: A, to: 'human', body: 'hi' });
  const one = timeline([], [start(1, 'context_full', 'note'), start(2, 'launch'), start(3, 'launch')]);
  assert.equal(one.length, 1);
  const item = (one[0] as { item: TranscriptItem }).item;
  assert.equal(item.id, 3);
  assert.deepEqual([item.session?.cause, item.session?.handoff], ['context_full', 'note']);
  // a message between two starts keeps them apart
  assert.equal(timeline([{ ...said, at: at(2) }], [start(1, 'launch'), start(3, 'launch')]).length, 3);
});

test('the live restart line says what happens now first, then why', () => {
  assert.equal(restartingLine({ cause: 'context_full', phase: 'writing' }), 'Writing the handoff to restart: context full');
  assert.equal(restartingLine({ cause: 'context_full', phase: 'starting' }), 'Starting a new session to restart: context full');
  assert.equal(
    restartingLine({ cause: 'settings_applied', phase: 'waiting', settings: ['model'] }),
    'Waiting for its turn to end, then restarts: model changed',
  );
  assert.equal(
    restartingLine({ cause: 'settings_applied', phase: 'starting', settings: ['model', 'effort'] }),
    'Starting a new session to restart: model, effort changed',
  );
  assert.equal(restartingLine({ cause: 'restart', phase: 'starting' }), 'Starting a new session to restart: restart asked');
});

test('turn minutes round down and say under a minute', () => {
  const now = Date.parse('2026-01-01T00:12:30Z');
  assert.equal(turnMinutes('2026-01-01T00:00:00Z', now), '12m');
  assert.equal(turnMinutes('2026-01-01T00:12:00Z', now), '<1m');
  assert.equal(turnMinutes(null, now), '<1m');
});

test("a task handed to an agent is drawn as the board's delivery, never as the person's own message", () => {
  const task = pm({ id: 5, from: 'human', to: A, kind: 'task', tasks: [{ id: 6477, title: 'Founder decisions' }] }); // local-ref-ok
  assert.equal(isTaskDelivery(task), true);
  assert.equal(sideOf(task, A, 'Handline desktop'), 'other');
  assert.equal(isTaskDelivery(pm({ from: 'human', to: A, kind: 'say' })), false);
  assert.equal(sideOf(pm({ from: 'human', to: A, kind: 'say' }), A, 'Handline desktop'), 'person');
});

test('foldBash folds an item into the message that sent it; an item with no loaded message stays its own row', () => {
  const at = '2026-01-01T10:00:00Z';
  const m = { id: 7, from: 'human', to: 'a', body: '!ls', at } as ChatMessage;
  const items = [
    { id: 1, at, kind: 'bash', command: 'ls', request: 7 },
    { id: 2, at, kind: 'bash', command: 'pwd' },
    { id: 3, at, kind: 'bash', command: 'old', request: 99 },
  ] as TranscriptItem[];
  const { entries, bash } = foldBash(timeline([m], items, 'mine', 'a'));
  assert.deepEqual(
    entries.map((e) => `${e.kind}:${e.id}`),
    ['message:7', 'item:2', 'item:3'],
  );
  assert.equal(bash.get(7)?.id, 1);
});

test('a ! line of unknown delivery is never advised to go again', () => {
  assert.doesNotMatch(maybeRanNote('!rm x'), /send it/i);
  assert.match(maybeRanNote('hello'), /send it again/);
});

test('a run of tool steps and session text is one record; a message or an answer to a request ends it', () => {
  const at = (s: number) => `2026-01-01T00:00:${String(s).padStart(2, '0')}Z`;
  const items = [
    item({ id: 10, at: at(1), kind: 'tool', tool: 'Bash', text: 'ls' }),
    item({ id: 12, at: at(2), kind: 'reply', text: 'Looking' }),
    item({ id: 14, at: at(2), kind: 'thinking', text: 'The test waits past the until' }),
    item({ id: 20, at: at(3), kind: 'tool', tool: 'Read', text: 'a.ts' }),
    item({ id: 31, at: at(5), kind: 'reply', text: 'Answer to a request' }),
    item({ id: 40, at: at(7), kind: 'tool', tool: 'Edit', text: 'a.ts' }),
  ];
  const out = foldTurns(timeline([msg({ id: 1, at: at(6), from: 'human', to: A })], items, 'mine', A, '', [], true));
  assert.deepEqual(
    out.map((e) => (e.kind === 'record' ? `record:${e.items.map((i) => i.id).join(',')}:${e.steps}` : `${e.kind}:${e.id}`)),
    ['record:10,12,14,20:2', 'item:31', 'message:1', 'record:40:1'],
  );
  // Without steps the agent's own chat keeps no tool line, as before.
  assert.equal(timeline([], items, 'mine', A).filter((e) => e.kind === 'item' && e.item.kind === 'tool').length, 0);
});

test('a task freed by its last blocker is a start card naming that blocker', () => {
  const ev = (over: object) =>
    item({ kind: 'task_event', text: undefined, task_event: { task: 8685, request: null, evidence: null, who: 'x', ...over } as never });
  assert.deepEqual(unblockedStart(ev({ event: 'unblocked', closed_task: 8632 })), {
    task: 8685,
    detail: 'Started: T8632, its last blocker, is done',
  });
  assert.equal(unblockedStart(ev({ event: 'unblocked' }))?.detail, 'Started: its last blocker is done');
  assert.equal(unblockedStart(ev({ event: 'done' })), null);
  assert.equal(unblockedStart(item({})), null);
});

test('who a message is from', () => {
  const mine = msg({ landing: 'typed', from: 'Handline desktop' });
  const ask = msg({ from: A, to: 'founder', state: 'queued', landing: 'delivered' });
  assert.equal(sideOf(mine, A, 'Handline desktop'), 'person');
  assert.equal(sideOf(ask, A, 'Handline desktop'), 'agent');
  assert.equal(sideOf(msg({ from: 'handline.auditor' }), A, 'Handline desktop'), 'other');
});

test('a task card is in the chat of the agent that made the change; the all tab shows every one', () => {
  const card = (id: number, who: string) =>
    ({
      id,
      at: `2026-01-01T00:00:0${id}Z`,
      kind: 'task_event',
      task_event: { event: 'changed', task: 7, request: null, evidence: null, who },
    }) as unknown as TranscriptItem;
  const items = [card(1, 'alice'), card(2, 'bob'), card(3, 'human')];
  assert.deepEqual(
    timeline([], items, 'mine', 'alice').map((e) => e.id),
    [1, 3],
  ); // the founder acts on the agent's behalf
  assert.deepEqual(
    timeline([], items, 'all', 'alice').map((e) => e.id),
    [1, 2, 3],
  );
});
