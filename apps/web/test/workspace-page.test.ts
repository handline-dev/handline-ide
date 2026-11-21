import type { AgentView, PendingRequest, Stats, Task } from '@handline/client';
import { expect, test } from 'vitest';
import { numbersOf, span } from '#src/workspace/WorkspacePage.tsx';
import { waitsOf } from '#src/workspace/WorkspaceGraph.tsx';

const agent = (key: string, state: string, status?: Partial<AgentView['status']>) =>
  ({
    key,
    name: key,
    state,
    status: status && { state: '', reason: '', text: '', until: null, since: null, tasks: [], ...status },
  }) as unknown as AgentView;
const task = (id: number, to: string, more: Partial<Task> = {}) =>
  ({ id, to, state: 'queued', done: null, waiting: null, ...more }) as Task;

test('an arrow is a wait: on you, or on the agent holding the task waited on; one outside the workspace draws none', () => {
  const agents = [agent('a', 'idle'), agent('b', 'blocked', { reason: 'task', tasks: [2, 9] }), agent('c', 'working')];
  const { edges } = waitsOf(agents, [task(2, 'c'), task(9, 'z')], new Set(['a', 'c']));
  expect(edges).toEqual([
    { from: 'a', to: 'you' },
    { from: 'c', to: 'you' },
    { from: 'b', to: 'c' },
  ]);
});

test('the numbers count this workspace only: landed in the last 7 days, blocked by cause, its share of the week', () => {
  const now = Date.parse('2026-11-21T12:00:00Z');
  const open = [
    task(1, 'a', { waiting: { on: 'founder', text: '', tasks: [], since: '' } }),
    task(2, 'a', { waiting: { on: 'task', text: '', tasks: [1], since: '' } }),
  ];
  const closed = [
    task(3, 'a', { state: 'done', done: '2026-11-20T00:00:00Z' }),
    task(4, 'a', { state: 'done', done: '2026-11-01T00:00:00Z' }),
  ];
  const stats = {
    your_wait_s: 1080,
    use: [
      { agent: 'a', week_pct: 12.4 },
      { agent: 'other', week_pct: 40 },
    ],
  } as Stats;
  const needs = [{ task: 1 }, { task: 5 }] as PendingRequest[];
  const n = numbersOf([agent('a', 'working')], open, closed, stats, needs, now);
  expect([n.landed, n.onYou, n.onTask, n.wait, n.week]).toEqual([1, 2, 1, 1080, 12.4]);
  expect([span(45), span(1080), span(7200)]).toEqual(['45s', '18m', '2h']);
});
