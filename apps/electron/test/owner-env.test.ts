import { test } from 'node:test';
import assert from 'node:assert/strict';
import { daemonEnv } from '#src/owner.ts';

test('the daemon gets the app environment without an agent session variables', () => {
  const env = daemonEnv({ HOME: '/h', PATH: '/bin', HANDLINE_AGENT: 'a5', HANDLINE_ACCOUNT: 'alpha', HANDLINE_HOME: '/hl' }, '/usr/bin');
  assert.deepEqual(env, { HOME: '/h', PATH: '/usr/bin', HANDLINE_HOME: '/hl' });
  assert.deepEqual(daemonEnv({ PATH: '/bin' }), { PATH: '/bin' });
});
