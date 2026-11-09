import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Task } from '@handline/client';
import { tabHasDot, tabTarget, type Turns } from '#src/index.ts';

const base = { me: 'ron', inboxUnread: 0 };
const task = (over: Partial<Turns & Pick<Task, 'unread'>> = {}): Turns & Pick<Task, 'unread'> => ({
  to: 'handline.ui',
  state: 'doing',
  acting: 'handline.ui',
  turns: [],
  ...over,
});

test('a chat tab stands on its agent; the plain Chat tab on the workspace it is in', () => {
  assert.deepEqual(tabTarget('chat', 'handline.ui'), { on: 'agent', name: 'handline.ui' });
  assert.deepEqual(tabTarget('chat:handline.qa', 'handline.ui'), { on: 'agent', name: 'handline.qa' });
  assert.equal(tabTarget('chat', null), null);
});

test('a task tab stands on its task, Requests on the inbox, every other kind on nothing', () => {
  assert.deepEqual(tabTarget('task:8503', null), { on: 'task', id: 8503 });
  assert.deepEqual(tabTarget('requests', null), { on: 'requests' });
  for (const kind of [
    undefined,
    'shell',
    'shell:h1',
    'diff',
    'tasks',
    'markdown',
    'web:http://x',
    'file:a.md',
    'usage',
    'preferences',
    'task:',
    'task:x',
  ])
    assert.equal(tabTarget(kind, 'handline.ui'), null, String(kind));
});

test('an agent has a dot only while it has unread messages', () => {
  const on = { on: 'agent', name: 'a' } as const;
  assert.equal(tabHasDot(on, base), false);
  assert.equal(tabHasDot(on, { ...base, agentUnread: 0 }), false);
  assert.equal(tabHasDot(on, { ...base, agentUnread: 2 }), true);
});

test('a task has a dot for an unread comment or the founder holding its turn', () => {
  const on = { on: 'task', id: 1 } as const;
  assert.equal(tabHasDot(on, base), false, 'not read yet');
  assert.equal(tabHasDot(on, { ...base, task: task() }), false);
  assert.equal(tabHasDot(on, { ...base, task: task({ unread: 1 }) }), true);
  const turn = { acting: 'human', turns: [{ agent: 'human' } as NonNullable<Task['turns']>[number]] };
  assert.equal(tabHasDot(on, { ...base, task: task(turn) }), true);
  assert.equal(tabHasDot(on, { ...base, task: task({ ...turn, state: 'done' }) }), false, 'a closed task is nobody’s turn');
});

test('Requests has a dot while the bell has unread items; other data does not light it', () => {
  const on = { on: 'requests' } as const;
  assert.equal(tabHasDot(on, { ...base, agentUnread: 3 }), false);
  assert.equal(tabHasDot(on, { ...base, inboxUnread: 1 }), true);
});
