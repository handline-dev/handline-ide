import { afterEach, expect, test, vi } from 'vitest';
import { uuid } from '#src/ui/uuid.ts';

const v4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

afterEach(() => vi.unstubAllGlobals());

test('makes a v4 id where randomUUID is missing (a page over plain http)', () => {
  vi.stubGlobal('crypto', { getRandomValues: globalThis.crypto.getRandomValues.bind(globalThis.crypto) });
  const a = uuid();
  expect(a).toMatch(v4);
  expect(uuid()).not.toBe(a);
});
