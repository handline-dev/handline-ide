import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { AgentView } from '@handline/client';
import { setGoingOf } from '#src/index.ts';

type A = Parameters<typeof setGoingOf>[0];
const since = '2026-01-01T00:00:00Z';
const need = (kind: string, source = 'daemon') => ({ kind, since, title: 't', text: null, source }) as AgentView['needs_you'];
const waiting = { cause: 'usage', from: 'alpha', phase: 'waiting', to: null, until: null, since } as const;
const go = (over: Partial<A>, act: string, login = false, spare = false) => setGoingOf({ state: 'idle', ...over }, act, login, spare);

const off = { state: 'off', reason: 'none', text: '', until: null, since, tasks: [] } as A['status'];
const clock = { state: 'waiting', reason: 'clock', text: '', until: null, since, tasks: [] } as A['status'];

test('a sleeping agent with no session is woken; a parked one, which may hold its session and wakes by its clock, is not', () => {
  assert.equal(go({ state: 'no_terminal' }, 'asleep'), 'wake');
  assert.equal(go({ state: 'closed', status: off }, 'asleep'), 'wake');
  assert.equal(go({ state: 'parked', status: clock }, 'asleep'), null);
});

test('a stopped agent: a failed start tries again, before a login', () => {
  assert.equal(go({ needs_you: need('start_failed') }, 'error', true), 'retry');
  assert.equal(go({ needs_you: need('no_session') }, 'error'), 'retry');
});

test('a need the daemon raised gets its fix though the daemon draws the agent as waiting for a person', () => {
  assert.equal(go({ state: 'stalled', needs_you: need('start_failed') }, 'you'), 'retry');
  assert.equal(go({ needs_you: need('trust') }, 'you'), 'terminal');
  assert.equal(go({ needs_you: need('dialog', 'hook') }, 'you'), null);
});

test('a stopped agent: its account to log in to, then the terminal for a dialog or a trust screen', () => {
  assert.equal(go({ needs_you: need('trust') }, 'error', true), 'login');
  assert.equal(go({ needs_you: need('trust') }, 'error'), 'terminal');
  assert.equal(go({ needs_you: need('dialog') }, 'error'), 'terminal');
});

test('a stalled agent the daemon raised nothing for is tried again; a need a hook raised is not the daemon one', () => {
  assert.equal(go({ state: 'stalled' }, 'error'), 'retry');
  const status = { state: 'stopped', reason: 'stalled', text: '', until: null, since, tasks: [] } as A['status'];
  assert.equal(go({ status }, 'error'), 'retry');
  assert.equal(go({ needs_you: need('dialog', 'hook') }, 'error'), null);
});

test('an agent on hold switches only while it waits for an account and another has room', () => {
  assert.equal(go({ account_switch: waiting }, 'held', false, true), 'switch');
  assert.equal(go({ state: 'parked', status: clock, account_switch: waiting }, 'asleep', false, true), 'switch');
  assert.equal(go({ state: 'no_terminal', account_switch: waiting }, 'asleep'), null);
  assert.equal(go({ account_switch: waiting }, 'held'), null);
  assert.equal(go({ account_switch: { ...waiting, phase: 'switching' } }, 'held', false, true), null);
  assert.equal(go({}, 'held', false, true), null);
});

test('an agent at work or starting is offered nothing; one at rest or waiting for an answer, nothing unless the daemon raised a need', () => {
  for (const act of ['working', 'waking'])
    assert.equal(go({ state: 'stalled', needs_you: need('start_failed'), account_switch: waiting }, act, true, true), null);
  for (const act of ['resting', 'you']) assert.equal(go({ state: 'stalled', status: off }, act), null);
});
