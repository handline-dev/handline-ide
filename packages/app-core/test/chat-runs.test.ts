import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { ChatMessage, TranscriptItem } from '@handline/client';
import {
  drawsNothing,
  knownUpTo,
  onePerTask,
  scheduledText,
  taskRuns,
  type TaskCard,
  type TaskEvent,
  type TimelineEntry,
} from '#src/index.ts';

const A = 'handline.ui';
const at = (n: number) => `2026-01-01T00:00:0${n}Z`;
const msg = (id: number, over: Partial<ChatMessage>): TimelineEntry => ({
  kind: 'message',
  id,
  at: at(id),
  message: {
    id,
    at: at(id),
    state_at: at(id),
    from: 'human',
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
  } as ChatMessage,
});
const item = (id: number, over: Partial<TranscriptItem>): TimelineEntry => ({
  kind: 'item',
  id,
  at: at(id),
  item: { id, at: at(id), kind: 'reply', text: 'ok', ...over } as TranscriptItem,
});
const notice = (id: number, task: number, to = 'handline.lander') =>
  msg(id, {
    from: A,
    to,
    kind: 'task',
    body: `T${task} handed to ${to}`,
    tasks: [{ id: task, title: `t${task}` }] as ChatMessage['tasks'],
  });
const nameOf = (k: string) => k.split('.').at(-1)!;
const cards = (rows: ReturnType<typeof taskRuns>) => rows.filter((r): r is TaskCard => 'events' in r);

test('task notices in a row are one card, and a line that draws nothing between them does not cut it', () => {
  const rows = taskRuns([notice(1, 10), msg(2, { from: 'handline.lead', to: A }), notice(3, 11)], A, 'human', nameOf);
  assert.equal(rows.length, 2);
  assert.deepEqual(
    cards(rows)[0].events.map((e) => [e.id, e.detail]),
    [
      ['T10', 'Handed to lander'],
      ['T11', 'Handed to lander'],
    ],
  );
});

test('a line that draws something ends the card; a task comment is its task row', () => {
  const comment = item(3, {
    text: '',
    task_comment: { task: 10, memo: 1, who: 'human', kind: 'note', text: '\nlooks good\nmore' },
  } as Partial<TranscriptItem>);
  const rows = taskRuns([notice(1, 10), msg(2, { body: 'next' }), comment], A, 'human', nameOf);
  assert.equal(rows.length, 3);
  assert.equal(cards(rows)[1].events[0].detail, 'You: looks good');
});

test('a task change heard live goes in by its time, and one its notice already says is dropped', () => {
  const live: TaskEvent[] = [
    { key: 'l1', kind: 'handoff', id: 'T10', title: '', detail: '', at: at(2), to: 'handline.lander' },
    { key: 'l2', kind: 'done', id: 'T12', title: '', detail: '', at: at(4) },
  ];
  const rows = taskRuns([notice(1, 10), msg(3, { body: 'next' })], A, 'human', nameOf, live);
  assert.deepEqual(
    rows.map((r) => ('events' in r ? r.events.map((e) => e.key) : r.kind)),
    [['1:10'], 'message', ['l2']],
  );
});

test('a session a move started draws nothing: its move line says it', () => {
  assert.equal(drawsNothing(item(1, { text: '', session: { cause: 'move' } } as Partial<TranscriptItem>), A, 'human'), true);
  assert.equal(drawsNothing(item(1, { text: '', session: { cause: 'context_full' } } as Partial<TranscriptItem>), A, 'human'), false);
});

test('a task the person gave draws no line of its words; their own message keeps them', () => {
  const tasks = [{ id: 7, title: 'Back and Forward' }];
  assert.equal(drawsNothing(msg(1, { from: 'human', to: A, kind: 'task', tasks, body: 'Make it work' }), A, 'human'), true);
  assert.equal(drawsNothing(msg(2, { from: 'human', to: A, body: 'Make it work' }), A, 'human'), false);
});

test('a message typed on a schedule is drawn, by its own words; the rest of what the daemon sends is not', () => {
  const scheduled = msg(1, { from: 'schedule', to: A, body: '[schedule 30 9 * * 1-5] Read the open requests.' });
  assert.equal(drawsNothing(scheduled, A, 'human'), false);
  assert.equal(scheduledText(scheduled.kind === 'message' ? scheduled.message.body : ''), 'Read the open requests.');
  // a message that only happens to start like one keeps its words
  assert.equal(scheduledText('[scheduled] look'), '[scheduled] look');
  assert.equal(drawsNothing(msg(2, { from: 'schedule', to: A, body: '[schedule 0 9 * * *] ' }), A, 'human'), true);
  assert.equal(drawsNothing(msg(3, { from: 'sys', to: A, body: 'three tasks wait' }), A, 'human'), true);
  assert.equal(drawsNothing(msg(4, { from: 'web.lead', to: A, body: 'look' }), A, 'human'), true);
  // it stands between two task cards as a line of its own
  const rows = taskRuns(
    [notice(1, 10), msg(2, { from: 'schedule', to: A, body: '[schedule 0 9 * * *] look' }), notice(3, 11)],
    A,
    'human',
    nameOf,
  );
  assert.deepEqual(
    rows.map((r) => ('events' in r ? 'card' : r.kind)),
    ['card', 'message', 'card'],
  );
});

test('each task shows once, in the first card it came up in, with all its changes; a later card left empty goes', () => {
  const ev = (id: string, key: string): TaskEvent => ({ key, kind: 'edit', id, title: id, detail: '' });
  const rows = [
    { id: 'a', events: [ev('T1', '1'), ev('T2', '2')] },
    { id: 'line' },
    { id: 'b', events: [ev('T2', '3')] },
    { id: 'c', events: [ev('T1', '4')] },
  ];
  const out = onePerTask(rows);
  assert.deepEqual(
    out.map((r) => r.id),
    ['a', 'line'],
  );
  assert.deepEqual(
    (out[0] as { events: TaskEvent[] }).events.map((e) => e.key),
    ['1', '2', '3', '4'],
  );
});

test('the rows known before a render are those up to the last line drawn; a line folded into an earlier card counts from there', () => {
  const entries = [msg(1, {}), notice(2, 10), msg(3, {}), notice(4, 10)];
  const rows = onePerTask(taskRuns(entries, A, 'human', nameOf));
  assert.equal(knownUpTo(rows, entries, 'message:1'), 0);
  assert.equal(knownUpTo(rows, entries, 'message:3'), 2);
  assert.equal(knownUpTo(rows, entries, 'message:4'), 1); // the second notice joined the first card
  assert.equal(knownUpTo(rows, entries, 'message:99'), Infinity);
});
