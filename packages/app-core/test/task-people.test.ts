import { test } from 'node:test';
import assert from 'node:assert/strict';
import { assignGroups, involved, replyTargets, treeDistance } from '#src/index.ts';

const team = {
  agents: [
    { key: 'boss', reports_to: null },
    { key: 'pc', reports_to: 'boss' },
    { key: 'mobile', reports_to: 'boss' },
    { key: 'rev', reports_to: 'boss' },
    { key: 'pc-sub', reports_to: 'pc' },
    { key: 'pc-sub2', reports_to: 'pc-sub' },
    { key: 'pc-sub3', reports_to: 'pc-sub2' },
    { key: 'far', reports_to: null },
  ],
};
const task = {
  from: 'boss',
  to: 'pc',
  events: [
    { at: '2026-10-01T10:00:00Z', agent: 'pc', actor: null },
    { at: '2026-10-03T10:00:00Z', agent: 'rev', actor: { kind: 'agent', name: 'sys' } },
    { at: '2026-10-02T10:00:00Z', agent: 'mobile', actor: { kind: 'user', name: 'human' } },
  ],
};
const names = ['boss', 'pc', 'mobile', 'rev', 'pc-sub', 'pc-sub2', 'pc-sub3', 'far', 'me'];

test('distance counts steps along the reports-to tree', () => {
  assert.equal(treeDistance(team, 'pc', 'pc'), 0);
  assert.equal(treeDistance(team, 'pc', 'boss'), 1);
  assert.equal(treeDistance(team, 'pc', 'mobile'), 2);
  assert.equal(treeDistance(team, 'pc-sub3', 'mobile'), 5);
  assert.equal(treeDistance(team, 'pc', 'far'), Infinity);
});

test('the picker lists who acted most recent first, then the near, then the rest by name', () => {
  const g = assignGroups(task, team, names, 'me');
  assert.deepEqual(
    g.map((x) => x.key),
    ['acted', 'near', 'rest'],
  );
  assert.deepEqual(g[0]!.names, ['rev', 'mobile', 'boss']);
  assert.deepEqual(g[1]!.names, ['pc-sub', 'pc-sub2']);
  assert.deepEqual(g[2]!.names, ['far', 'pc-sub3']);
});

test('the holder, the viewer and the system never appear, and an empty group is left out', () => {
  const all = assignGroups(task, team, [...names, 'sys'], 'me').flatMap((g) => g.names);
  for (const n of ['pc', 'me', 'sys']) assert.ok(!all.includes(n));
  assert.deepEqual(
    assignGroups({ ...task, events: [] }, { agents: [] }, ['a'], 'me').map((g) => g.key),
    ['rest'],
  );
});

test('reply chips are the people the task names, latest first, without the viewer', () => {
  assert.deepEqual(involved(task), ['rev', 'mobile', 'human', 'pc', 'boss']);
  assert.deepEqual(replyTargets(task, 'boss'), ['rev', 'mobile', 'pc']);
});

test('the close list is short: only the nearest few, the rest wait with the other teams', () => {
  const many = {
    agents: [
      { key: 'boss', reports_to: null },
      ...Array.from({ length: 10 }, (_, i) => ({ key: `k${String(i).padStart(2, '0')}`, reports_to: 'boss' })),
    ],
  };
  const t = { from: 'boss', to: 'boss', events: [] };
  const g = assignGroups(t, many, [], 'me');
  assert.equal(g.find((x) => x.key === 'near')!.names.length, 6);
  assert.equal(g.find((x) => x.key === 'rest')!.label, 'Other teams');
  assert.equal(g.find((x) => x.key === 'rest')!.names.length, 4);
});
