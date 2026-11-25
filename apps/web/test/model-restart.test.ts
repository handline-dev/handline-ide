import { expect, it } from 'vitest';
import { restartWords } from '#src/chat/agent/ModelChip.tsx';

it('an effort change says it hands off and starts a new conversation on the new effort', () => {
  const w = restartWords({ effort: 'max' });
  expect(w.title).toBe('Change effort to max?');
  expect(w.message).toBe('When it is next idle, it hands off and starts a new conversation on max effort, without the cache it has now.');
  expect(w.confirm).toBe('Change');
});

it('a model change, alone or with an effort, names what it switches to', () => {
  expect(restartWords({ model: 'sonnet' }).title).toBe('Switch to sonnet?');
  const both = restartWords({ model: 'sonnet', effort: 'low' });
  expect(both.title).toBe('Switch to sonnet at low effort?');
  expect(both.confirm).toBe('Switch');
});
