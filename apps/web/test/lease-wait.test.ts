import { expect, test } from 'vitest';
import { leaseWaits } from '#src/tools/leaseWait.ts';

const run = {
  kind: 'mac:ios-build',
  name: 'ios-build',
  scope: 'mac',
  cmds: [],
  at_once: 1,
  why: '',
  set_by: 'a1',
  set_at: '',
  held: [{ agent: 'a5', since: 's', hold: false }],
  waiting: [
    { agent: 'a10', since: '', position: 2, task: 102 },
    { agent: 'a9', since: '', position: 1, task: 101 },
    { agent: 'a11', since: '', position: 3 },
  ],
};

test('a task in line for a run gets the run name, its place, and the line holder-first then in order; a port wait is its own', () => {
  const nameOf = (k: string) => ({ a5: 'daemon-core', a9: 'ui', a10: 'web' })[k] ?? k;
  const port = { kind: 'port', max: 1, held: [], queue: [{ agent: 'a3', since: '', position: 1, says: '', task: 7 }] };
  const m = leaseWaits([run], [port], nameOf);
  const line = [
    { key: 'a9', name: 'ui', task: 101, position: 1 },
    { key: 'a10', name: 'web', task: 102, position: 2 },
    { key: 'a11', name: 'a11', task: undefined, position: 3 },
  ];
  const holders = [{ key: 'a5', name: 'daemon-core', since: 's' }];
  expect(m.get(101)).toEqual({ kind: 'run', run: 'ios-build', position: 1, holders, waiting: line });
  expect(m.get(102)).toEqual({ kind: 'run', run: 'ios-build', position: 2, holders, waiting: line });
  expect(m.get(7)).toEqual({ kind: 'port' });
  expect(m.size).toBe(3);
});
