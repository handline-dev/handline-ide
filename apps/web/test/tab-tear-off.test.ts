import { expect, test } from 'vitest';
import { nearBar, TEAR_OFF } from '#src/ui/dnd.tsx';

const body = { getBoundingClientRect: () => ({ top: 100 }) } as HTMLElement;

test('a drag just under the tab bar stays on the bar', () => {
  expect(nearBar({ clientY: 100 }, body)).toBe(true);
  expect(nearBar({ clientY: 100 + TEAR_OFF - 1 }, body)).toBe(true);
});

test('a drag past the band reaches the pane', () => {
  expect(nearBar({ clientY: 100 + TEAR_OFF }, body)).toBe(false);
  expect(nearBar({ clientY: 400 }, body)).toBe(false);
});
