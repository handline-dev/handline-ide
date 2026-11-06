import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Task } from '@handline/client';
import { foundersTurn, turnChain } from '#src/turns.ts';

const turn = (agent: string) =>
  ({ agent, ask: '', by: '', since: '', on_pass: null, evidence: null }) as NonNullable<Task['turns']>[number];

test('the chain reads from the top turn down to the owner, the founder by role', () => {
  const t = { to: 'handline.ui', state: 'doing', acting: 'human', turns: [turn('handline'), turn('human')] };
  assert.equal(turnChain(t, 'ron'), 'founder ← handline ← handline.ui');
  assert.equal(foundersTurn(t, 'ron'), true);
});

test('no turns: no chain, and the founder as owner has no turn to give back', () => {
  const t = { to: 'human', state: 'doing', acting: 'human', turns: [] };
  assert.equal(turnChain(t, 'ron'), '');
  assert.equal(foundersTurn(t, 'ron'), false);
});
