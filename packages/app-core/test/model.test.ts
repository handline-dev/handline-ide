import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { AgentView, Prompt } from '@handline/client';
import { promptChoices, whyOf, dotOf, offered, splitCancel, checkedKeys, queuedBehind } from '#src/index.ts';

const agent = (name: string, over: Partial<AgentView> = {}): AgentView =>
  ({
    key: name,
    name,
    role: 'implementer',
    model: 'opus',
    effort: null,
    state: 'idle',
    state_since: new Date().toISOString(),
    account: 'alpha',
    task: null,
    cache_until: null,
    has_dialog: false,
    parent: null,
    ...over,
  }) as AgentView;

test('the dot follows the daemon state, with a dialog taking over and a stall blocked', () => {
  assert.equal(dotOf(agent('a', { state: 'running' })), 'running');
  assert.equal(dotOf(agent('a', { state: 'running', has_dialog: true })), 'waiting');
  assert.equal(dotOf(agent('a', { state: 'stalled' })), 'blocked');
  assert.equal(dotOf(agent('a', { state: 'no_terminal' })), 'none');
  assert.equal(dotOf(agent('a', { state: 'parked' })), 'parked');
  assert.equal(dotOf(agent('a', { state: 'something_new' })), 'idle');
  const handoff = (phase: 'writing' | 'starting') => ({ handoff: { cause: 'context_full', phase, since: '' } });
  assert.equal(dotOf(agent('a', { state: 'idle', ...handoff('starting') })), 'waking');
  assert.equal(dotOf(agent('a', { state: 'idle', ...handoff('writing') })), 'idle');
  assert.equal(dotOf(agent('a', { state: 'running', ...handoff('starting') })), 'waking');
});

test('queued behind is the owned tasks less the current one, never negative, null with no count', () => {
  assert.equal(queuedBehind(agent('x')), null);
  assert.equal(queuedBehind(agent('x', { task_count: 4, task: { id: 7, title: 't' } })), 3);
  assert.equal(queuedBehind(agent('x', { task_count: 2 })), 2); // no current task: all wait
  assert.equal(queuedBehind(agent('x', { task_count: 0, task: { id: 7, title: 't' } })), 0);
});

test('a permission card offers only yes and no; other dialogs offer everything', () => {
  const p = (kind: string): Prompt => ({
    id: '1',
    agent: 'a',
    kind,
    lines: [],
    since: '',
    options: [
      { key: '1', label: 'Yes', answer: 'yes' },
      { key: '2', label: 'Yes, always', answer: 'other' },
      { key: '3', label: 'No', answer: 'no' },
    ],
  });
  assert.deepEqual(
    offered(p('permission')).map((o) => o.key),
    ['1', '3'],
  );
  assert.equal(offered(p('trust')).length, 3);
  // The daemon's own Always allow for an exact command carries its rule, and is offered; a screen's "always" without one is not.
  const hook: Prompt = {
    ...p('permission'),
    options: [
      { key: 'yes', label: 'Allow once', answer: 'yes' },
      { key: 'always', label: 'Always allow lander Bash(npm test)', answer: 'yes', rule: 'Bash(npm test)' },
      { key: 'yes2', label: 'Yes, always', answer: 'yes' },
      { key: 'no', label: 'Deny', answer: 'no' },
    ],
  };
  assert.deepEqual(
    offered(hook).map((o) => o.key),
    ['yes', 'always', 'no'],
  );
});

const opt = (key: string, label: string, answer: 'yes' | 'no' | 'other' = 'other') => ({ key, label, answer, highlighted: false });
const card = (kind: string) =>
  ({
    id: 'p',
    agent: 'a',
    kind,
    tool: 'Bash',
    command: null,
    reason: null,
    lines: [],
    since: null,
    options: [opt('1', 'Yes', 'yes'), opt('2', 'Yes, always', 'yes'), opt('3', 'No', 'no')],
  }) as unknown as Prompt;

test('a card offers a permission yes and no only, a dialog its always options only when a rule can be kept, and a question everything', () => {
  const keys = (p: Prompt, rules: boolean) => promptChoices(p, rules).map((o) => o.key);
  assert.deepEqual(keys(card('permission'), true), ['1', '3']);
  assert.deepEqual(keys(card('dialog'), true), ['1', '2', '3']);
  assert.deepEqual(keys(card('dialog'), false), ['1', '3']);
  assert.deepEqual(keys(card('question'), false), ['1', '2', '3']);
});

test('a warning mark says why: the wall, else the state detail, else nothing', () => {
  assert.equal(whyOf({ wall_reason: ' the account is out of usage ', state_detail: 'x' }), 'the account is out of usage');
  assert.equal(whyOf({ state_detail: 'waits for the build' }), 'waits for the build');
  assert.equal(whyOf({ state_detail: '' }), null);
});

test('a question keeps Cancel out of its options, and a multi-select submits the checked keys in the order listed', () => {
  const q = card('question');
  const choices = [
    { key: '1', label: 'a', answer: 'other' as const },
    { key: '2', label: 'b', answer: 'other' as const },
    { key: '3', label: 'c', answer: 'other' as const },
    { key: 'esc', label: 'Cancel (Esc)', answer: 'other' as const },
  ];
  const { cancel, opts } = splitCancel(q, choices);
  assert.equal(cancel?.key, 'esc');
  assert.deepEqual(
    opts.map((o) => o.key),
    ['1', '2', '3'],
  );
  assert.equal(splitCancel(card('dialog'), choices).cancel, undefined);
  assert.deepEqual(checkedKeys(opts, ['3', '1']), ['1', '3']);
  assert.deepEqual(checkedKeys(opts, []), []);
});
