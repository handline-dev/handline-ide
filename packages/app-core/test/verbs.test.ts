import { test } from 'node:test';
import assert from 'node:assert/strict';
import { VERBS, callOf } from '#src/index.ts';

test('builds each call from the verb', () => {
  assert.deepEqual(callOf(VERBS.close, 'a.b'), { method: 'session.stop', params: { agent: 'a.b', mode: 'close' } });
  assert.deepEqual(callOf(VERBS.restart_fresh, 'a.b'), { method: 'session.restart', params: { agent: 'a.b', fresh: true } });
  assert.deepEqual(callOf(VERBS.move, 'a.b', 'beta'), { method: 'agent.move', params: { agent: 'a.b', account: 'beta' } });
});
