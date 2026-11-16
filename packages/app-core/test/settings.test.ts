import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mayChange, problemWith, type Field } from '#src/index.ts';

test('the founder changes any field; anyone else only the keys an agent may change on itself', () => {
  assert.equal(mayChange('repo', 'owner'), true);
  assert.equal(mayChange('repo', 'member'), false);
  assert.equal(mayChange('sandbox', 'agent'), false);
  for (const k of ['model', 'effort', 'context_k', 'handoff_note'] as const) assert.equal(mayChange(k, 'agent'), true);
});

test('a number field takes whole numbers only, and says what is allowed', () => {
  const contextK: Field = { key: 'context_k', label: 'hand off at', section: 'model', kind: 'number' };
  const idle: Field = { key: 'idle_close', label: 'idle close', section: 'browser', kind: 'number' };
  const model: Field = { key: 'model', label: 'model', section: 'model', kind: 'model' };
  assert.equal(problemWith(contextK, '300'), null);
  assert.match(problemWith(contextK, 'abc')!, /whole number/);
  assert.match(problemWith(contextK, '0')!, /at least 1/);
  assert.equal(problemWith(idle, '0'), null);
  assert.match(problemWith(idle, '1.5')!, /whole minutes/);
  assert.match(problemWith(model, '  ')!, /model name/);
});
