import { expect, test } from 'vitest';
import { unreachableText } from '@handline/app-react';

test('the first tries say the daemon is starting, with no address', () => {
  expect(unreachableText(1, 'ws://127.0.0.1:7443/rpc', new Error('closed'))).toBe('Starting the daemon…');
  expect(unreachableText(10, 'ws://127.0.0.1:7443/rpc', new Error('closed'))).toBe('Starting the daemon…');
});

test('later tries name the address, the reason, and what to do', () => {
  const t = unreachableText(11, 'ws://127.0.0.1:7443/rpc', new Error('the daemon took this app as guest'));
  expect(t).toContain('ws://127.0.0.1:7443/rpc');
  expect(t).toContain('the daemon took this app as guest');
  expect(t).toContain('Start Handline');
});

test('the address is said once when the reason already names it', () => {
  const where = 'ws://127.0.0.1:7443/rpc';
  const t = unreachableText(11, where, new Error(`cannot connect to ${where}`));
  expect(t.split(where)).toHaveLength(2);
});
