import { test } from 'node:test';
import assert from 'node:assert/strict';
import { changeRows, decidedTitle, proposalTitle } from '#src/index.ts';

const nameOf = (k: string) => ({ a10: 'ui' })[k] ?? k;

test('an HR proposal is called by what it does, with the name', () => {
  assert.equal(proposalTitle({ kind: 'context', agent: 'a10', title: 'context a10', diff: '' }, nameOf), "Change ui's prompt");
  assert.equal(proposalTitle({ kind: 'bundle', agent: '3 changes', title: 'bundle 3 changes', diff: 'a\nb\nc' }, nameOf), '3 changes');
  assert.equal(proposalTitle({ kind: 'mystery', agent: 'a10', title: 'mystery a10', diff: '' }, nameOf), 'mystery a10');
});

test('a bundle is called by its steps', () => {
  const p = {
    kind: 'bundle',
    agent: '2 changes',
    title: 'bundle',
    diff: 'a\nb',
    changes: [
      { kind: 'settings', agent: 'a10' },
      { kind: 'context', agent: 'a10' },
    ],
  };
  assert.equal(proposalTitle(p, nameOf), "Change ui's settings, change ui's prompt");
  const three = {
    ...p,
    changes: [
      { kind: 'retire', agent: 'a4' },
      { kind: 'context', agent: 'a3' },
      { kind: 'context', agent: 'a8' },
    ],
  };
  assert.equal(proposalTitle(three, nameOf), 'Retire a4, change 2 prompts');
});

test('change lines read as what, from and to', () => {
  assert.deepEqual(changeRows('model: sonnet -> opus'), [{ what: 'Model', from: 'sonnet', to: 'opus' }]);
  assert.deepEqual(changeRows('effort: high'), [{ what: 'Effort', to: 'high' }]);
  assert.deepEqual(changeRows('set model of handline.auditor to opus'), [{ what: 'Model', to: 'opus' }]);
  assert.deepEqual(changeRows('run: git push\ncwd: /x'), [{ text: 'run: git push' }, { text: 'cwd: /x' }]);
  assert.deepEqual(changeRows('retire agent handline.term-build'), [{ text: 'retire agent handline.term-build' }]);
});

test('a decided command run is called by its Why, ids as names, never by the command cut at an id', () => {
  const plain = (t: string) => t.replace(/<@(a\d+)>/g, (_, k: string) => nameOf(k));
  const run = { kind: 'run', agent: 'a10', title: 'run in /work, with its output told to <@a10>', diff: 'run: ls' };
  assert.equal(decidedTitle({ ...run, reason: 'The push needs <@a10> keychain.\nMore.' }, nameOf, plain), 'The push needs ui keychain.');
  assert.equal(decidedTitle({ ...run, reason: null }, nameOf, plain), 'Run a command');
  assert.equal(decidedTitle({ kind: 'mystery', agent: 'a10', title: 'mystery <@a10>', diff: '' }, nameOf, plain), 'mystery ui');
});
