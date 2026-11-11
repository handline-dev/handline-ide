import { test } from 'node:test';
import assert from 'node:assert/strict';
import { byWorkspace, nameOf, renderRefs, toRefs, type Directory } from '#src/index.ts';

const dir: Directory = {
  agents: [
    { key: 'a1', name: 'lead', workspace: 'w1' },
    { key: 'a2', name: 'designer', workspace: 'w1' },
    { key: 'a3', name: 'lead', workspace: 'w2' },
    { key: 'a4', name: 'designer', workspace: 'w2' },
    { key: 'a5', name: 'auditor', workspace: 'w2' },
  ],
  people: [{ key: 'p1', display_name: 'Sam', handle: null, role: 'owner' }],
  workspaces: [
    { key: 'w1', name: 'shop', folder: '/s', added_folders: [], lead: 'a1' },
    { key: 'w2', name: 'site', folder: '/t', added_folders: [], lead: 'a3' },
  ],
};

test('a key reads as a lead by its workspace, an agent by its name or workspace.name when two share it, a person by display name', () => {
  assert.deepEqual(
    ['a1', 'a2', 'a3', 'a4', 'a5', 'p1', 'w2', 'sys', 'a99'].map((k) => nameOf(dir, k)),
    ['shop', 'shop.designer', 'site', 'site.designer', 'auditor', 'Sam', 'site', 'task-board', 'a99'],
  );
});

test('a stored mention reads as @name, and a typed @word that names someone is sent as its key', () => {
  assert.equal(renderRefs(dir, 'ask <@a5> and <@p1>, not <@zz>'), 'ask @auditor and @Sam, not @zz');
  assert.equal(toRefs(dir, '@auditor and @site.designer. Mail a@b, @nobody'), '<@a5> and <@a4>. Mail a@b, @nobody');
  assert.equal(toRefs(dir, renderRefs(dir, 'ping <@a3> and <@p1>')), 'ping <@a3> and <@p1>');
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
