import { test } from 'node:test';
import assert from 'node:assert/strict';
import { proposalTitle } from '#src/index.ts';

const nameOf = (k: string) => ({ a10: 'ui' })[k] ?? k;

test('an HR proposal is called by what it does, with the name', () => {
  assert.equal(proposalTitle({ kind: 'context', agent: 'a10', title: 'context a10', diff: '' }, nameOf), "Change ui's prompt");
  assert.equal(proposalTitle({ kind: 'bundle', agent: '3 changes', title: 'bundle 3 changes', diff: 'a\nb\nc' }, nameOf), '3 changes');
  assert.equal(proposalTitle({ kind: 'mystery', agent: 'a10', title: 'mystery a10', diff: '' }, nameOf), 'mystery a10');
});
