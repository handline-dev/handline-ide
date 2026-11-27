import { test } from 'node:test';
import assert from 'node:assert/strict';
import { byWorkspace, nameOf, plainRefs, renderRefs, toRefs, type Directory } from '#src/index.ts';

const dir: Directory = {
  agents: [
    { key: 'a1', name: 'lead', workspace: 'w1' },
    { key: 'a2', name: 'designer', workspace: 'w1' },
    { key: 'a3', name: 'lead', workspace: 'w2' },
    { key: 'a4', name: 'designer', workspace: 'w2' },
    { key: 'a5', name: 'auditor', workspace: 'w2' },
  ],
  people: [{ key: 'p1', display_name: 'Sam', handle: null, role: 'owner', removed: false }],
  workspaces: [
    { key: 'w1', name: 'shop', folder: '/s', added_folders: [], folders: [], lead: 'a1', next_lead: 'a2' },
    { key: 'w2', name: 'site', folder: '/t', added_folders: [], folders: [], lead: 'a3', next_lead: 'a4' },
  ],
};

test('a key reads as an agent by its name, or workspace/name when two share it, a person by display name', () => {
  assert.deepEqual(
    ['a1', 'a2', 'a3', 'a4', 'a5', 'p1', 'w2', 'sys', 'a99'].map((k) => nameOf(dir, k)),
    ['shop/lead', 'shop/designer', 'site/lead', 'site/designer', 'auditor', 'Sam', 'site', 'task-board', 'a removed agent'],
  );
});

test('a seat (hired_from) reads as the agent it works for', () => {
  const seats = { ...dir, agents: [...dir.agents, { key: 'a7', name: 'a7', workspace: 'w2', hired_from: 'a5' }] };
  assert.equal(nameOf(seats, 'a7'), 'auditor');
});

test('inside its own workspace an agent reads by its name alone', () => {
  assert.deepEqual(
    ['a1', 'a2', 'a3'].map((k) => nameOf(dir, k, 'w1')),
    ['lead', 'designer', 'site/lead'],
  );
});

test('a stored mention reads as @name, and a typed @word that names someone is sent as its key', () => {
  assert.equal(renderRefs(dir, 'ask <@a5> and <@p1>, not <@zz>'), 'ask @auditor and @Sam, not @zz');
  // The person reading is You.
  assert.equal(renderRefs({ ...dir, me: 'p1' }, 'ask <@p1> and <@a5>'), 'ask @You and @auditor');
  assert.equal(toRefs(dir, '@auditor and @site/designer. Mail a@b, @nobody'), '<@a5> and <@a4>. Mail a@b, @nobody');
  assert.equal(toRefs(dir, renderRefs(dir, 'ping <@a3> and <@p1>')), 'ping <@a3> and <@p1>');
  assert.equal(toRefs(dir, 'at /work/@auditor/notes.md and ~@auditor'), 'at /work/@auditor/notes.md and ~@auditor');
});

test('agents group by workspace in its order, lead first, and an agent of an unknown workspace comes last', () => {
  const g = byWorkspace(dir, [
    { key: 'a4', workspace: 'w2' },
    { key: 'a3', workspace: 'w2' },
    { key: 'a9', workspace: 'w9' },
    { key: 'a2', workspace: 'w1' },
  ]);
  assert.deepEqual(
    g.map((x) => [x.workspace?.name ?? null, x.agents.map((a) => a.key)]),
    [
      ['shop', ['a2']],
      ['site', ['a3', 'a4']],
      [null, ['a9']],
    ],
  );
});

test('a key no list holds is never shown raw, and plain text names without @', () => {
  assert.equal(nameOf(dir, 'a404'), 'a removed agent');
  assert.equal(nameOf(dir, 'p77'), 'someone');
  assert.equal(plainRefs(dir, 'retire <@a404>, replace role of <@a5>'), 'retire a removed agent, replace role of auditor');
});

test('a bare key inside daemon text reads as its name, and only a key the lists hold', () => {
  assert.equal(renderRefs(dir, 'Stalled: 3 starts for a5 failed'), 'Stalled: 3 starts for auditor failed');
  assert.equal(renderRefs(dir, 'held by @a5, see p1'), 'held by auditor, see Sam');
  assert.equal(plainRefs(dir, 'retire <@a5> after a1'), 'retire auditor after shop/lead');
  assert.equal(renderRefs(dir, 'a99 and file a5.txt and x/a5 stay'), 'a99 and file a5.txt and x/a5 stay');
});
