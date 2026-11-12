import { test } from 'node:test';
import assert from 'node:assert/strict';
import { assignGroups, involved, replyTargets } from '#src/index.ts';

const team = [
  { key: 'boss', workspace: 'w1' },
  { key: 'pc', workspace: 'w2' },
  { key: 'mobile', workspace: 'w1' },
  { key: 'rev', workspace: 'w1' },
  { key: 'pc-sub', workspace: 'w2' },
  { key: 'pc-sub2', workspace: 'w2' },
  { key: 'pc-sub3', workspace: 'w1' },
  { key: 'far', workspace: 'w3' },
];
const task = {
  from: 'boss',
  to: 'pc',
  events: [
    { at: '2026-10-01T10:00:00Z', agent: 'pc', actor: null },
    { at: '2026-10-03T10:00:00Z', agent: 'rev', actor: { kind: 'agent', name: 'sys' } },
    { at: '2026-10-02T10:00:00Z', agent: 'mobile', actor: { kind: 'user', name: 'human' } },
  ],
};

test("the picker lists who acted most recent first, then the holder's and opener's workspaces by name, then the rest by name", () => {
  const g = assignGroups(task, team, 'me');
  assert.deepEqual(
    g.map((x) => x.key),
    ['acted', 'near', 'rest'],
  );
  assert.deepEqual(g[0]!.names, ['rev', 'mobile', 'boss']);
  assert.deepEqual(g[1]!.names, ['pc-sub', 'pc-sub2', 'pc-sub3']);
  assert.deepEqual(g[2]!.names, ['far']);
});

test('the holder, the viewer and the system never appear, and an empty group is left out', () => {
  const all = assignGroups(task, [...team, { key: 'me', workspace: 'w2' }, { key: 'sys', workspace: 'w2' }], 'me').flatMap((g) => g.names);
  for (const n of ['pc', 'me', 'sys']) assert.ok(!all.includes(n));
  assert.deepEqual(
    assignGroups({ ...task, events: [] }, [{ key: 'a', workspace: 'w9' }], 'me').map((g) => g.key),
    ['rest'],
  );
});

test('reply chips are the people the task names, latest first, without the viewer', () => {
  assert.deepEqual(involved(task), ['rev', 'mobile', 'human', 'pc', 'boss']);
  assert.deepEqual(replyTargets(task, 'boss'), ['rev', 'mobile', 'pc']);
});

test("the workspace's list is short: only the first few by name, the rest come after", () => {
  const many = [
    { key: 'boss', workspace: 'w1' },
    ...Array.from({ length: 10 }, (_, i) => ({ key: `k${String(i).padStart(2, '0')}`, workspace: 'w1' })),
  ];
  const t = { from: 'boss', to: 'boss', events: [] };
  const g = assignGroups(t, many, 'me');
  assert.equal(g.find((x) => x.key === 'near')!.names.length, 6);
  assert.equal(g.find((x) => x.key === 'rest')!.label, 'Other teams');
  assert.equal(g.find((x) => x.key === 'rest')!.names.length, 4);
});
