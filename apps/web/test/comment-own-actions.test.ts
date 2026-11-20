import { expect, it } from 'vitest';
import { ownComment } from '#src/tools/TaskEvents.tsx';

it("only the founder's own comment can be edited or deleted", () => {
  expect(ownComment({ memo: 7 }, true, 1)).toEqual({ task: 1, memo: 7, deletable: true });
  expect(ownComment({ memo: 7 }, false, 1)).toBeNull();
});

it('a comment that passed a turn can be edited, not deleted', () =>
  expect(ownComment({ memo: 7, turn: 'push' }, true, 1)?.deletable).toBe(false));
