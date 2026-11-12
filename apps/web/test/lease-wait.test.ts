import { expect, test } from 'vitest';
import { leaseWaits } from '#src/tools/leaseWait.ts';

const held = (agent: string, reserved_for?: number) => ({
  lease: 1,
  agent,
  since: '',
  hold: false,
  pids: [],
  ...(reserved_for ? { reserved_for } : {}),
});

test('a task in line says which resource and who uses it now, and how many are ahead; a held-open call and a reservation are not users', () => {
  const nameOf = (k: string) => ({ a5: 'daemon-core', a6: 'daemon-tasks' })[k] ?? k;
  const m = leaseWaits(
    [
      {
        kind: 'simulator',
        max: 1,
        held: [held('a5'), held('a6', 77)],
        queue: [
          { agent: 'a9', since: '', position: 1, says: '', task: 101 },
          { agent: 'a10', since: '', position: 2, says: '', task: 102 },
          { agent: 'a11', since: '', position: 3, says: '' },
        ],
      },
    ],
    nameOf,
  );
  expect([...m]).toEqual([
    [101, { kind: 'simulator', text: 'Waiting for a simulator · in use by daemon-core' }],
    [102, { kind: 'simulator', text: 'Waiting for a simulator · in use by daemon-core · 1 ahead' }],
  ]);
});
