import { cacheRewriteUsd } from '@handline/app-core';
import { expect, it } from 'vitest';
import { restartWords } from '#src/chat/agent/ModelChip.tsx';

it('an effort change asks with its own words, and costs the context on the model the agent is on', () => {
  const w = restartWords({ effort: 'max' }, 'opus', 74_000);
  expect(w.title).toBe('Change effort to max');
  expect(w.compactTitle).toBe('Compact, then change');
  expect(w.cancel).toBe('Keep the current effort.');
  expect(w.keep).toContain(`74k tokens of context are written into the cache once, about $${cacheRewriteUsd(74_000, 'opus').toFixed(2)}`);
  expect(w.keep).toContain('on max effort');
});

it('a model change, alone or with an effort, names what it switches to and costs the new model', () => {
  expect(restartWords({ model: 'sonnet' }, 'opus', null).title).toBe('Switch to sonnet');
  const both = restartWords({ model: 'sonnet', effort: 'low' }, 'opus', 10_000);
  expect(both.title).toBe('Switch to sonnet at low effort');
  expect(both.compactTitle).toBe('Compact, then switch');
  expect(both.keep).toContain(`about $${cacheRewriteUsd(10_000, 'sonnet').toFixed(2)}`);
});
