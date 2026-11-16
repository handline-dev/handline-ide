import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { AgentView, Worktree } from '@handline/client';
import { DEFAULT_NAV, movePin, navDock, navOf, togglePin, withOpen } from '#src/index.ts';

const agent = (name: string, state: string, parent: string | null, unread = false) =>
  ({ key: name, name, state, parent, role: '', founder_unread: unread ? 1 : 0 }) as unknown as AgentView;
const team = [
  agent('handline', 'idle', null),
  agent('handline.pc', 'running', 'handline'),
  agent('handline.daemon', 'waiting', 'handline', true),
  agent('handline.daemon.auditor', 'parked', 'handline.daemon'),
  agent('orca', 'idle', null, true),
];

test('missing prefs read as the defaults', () => {
  assert.deepEqual(navOf(undefined), DEFAULT_NAV);
  assert.deepEqual(navOf({ nav: { ...DEFAULT_NAV, layout: 'flat' } }).layout, 'flat');
});

test('the dock is the pins, then the open tabs that are not pinned, each with the branch of the worktree it shows', () => {
  const trees = [
    { path: '/w/pc', owner: 'handline.pc', branch: 'pc-main' },
    { path: '/w/spare', owner: null, branch: 'spare' },
    { path: '/w/orca', owner: 'orca', branch: 'orca-main' },
  ] as Worktree[];
  const d = navDock(
    team,
    ['orca'],
    [{ agent: 'handline.pc', path: '/w/spare' }, { agent: 'orca' }, { agent: 'gone' }, { agent: 'handline' }],
    trees,
  );
  assert.deepEqual(
    [d.pinned, d.open].map((l) => l.map((i) => `${i.agent.key}@${i.branch}`)),
    [['orca@orca-main'], ['handline.pc@spare', 'handline@null']],
  );
});

test('pins toggle and move; opening past the limit drops the oldest', () => {
  assert.deepEqual(togglePin(['a'], 'b'), ['a', 'b']);
  assert.deepEqual(togglePin(['a', 'b'], 'a'), ['b']);
  assert.deepEqual(movePin(['a', 'b', 'c'], 'c', 0), ['c', 'a', 'b']);
  assert.deepEqual(movePin(['a', 'b'], 'x', 0), ['a', 'b']);
  const tabs = (...names: string[]) => names.map((agent) => ({ agent }));
  assert.deepEqual(withOpen(tabs('a', 'b'), { agent: 'a', path: '/w' }), [{ agent: 'a', path: '/w' }, { agent: 'b' }]);
  assert.deepEqual(withOpen(tabs('a', 'b', 'c'), { agent: 'd' }, [], 3), tabs('b', 'c', 'd'));
  assert.deepEqual(withOpen(tabs('p', 'b', 'c'), { agent: 'd' }, ['p'], 2), tabs('p', 'c', 'd'));
});
