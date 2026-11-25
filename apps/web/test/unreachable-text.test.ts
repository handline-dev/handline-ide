import { expect, test } from 'vitest';
import { unreachableText } from '@handline/app-react';

test('a normal start says nothing about the connection', () => {
  expect(unreachableText(0)).toBeNull();
  expect(unreachableText(3999)).toBeNull();
});

test('a longer wait says one plain line, with no address', () => {
  expect(unreachableText(4000)).toBe('Connecting to Handline…');
  expect(unreachableText(600_000)).toBe('Connecting to Handline…');
});
