import { test } from 'node:test';
import assert from 'node:assert/strict';
import { PAGE_SETTINGS } from '#src/shell.ts';
import { DEFAULT_LIMITS } from '#src/resources.ts';

test('the shell forwards every limit the status bar resource area writes', () => {
  for (const key of [...Object.keys(DEFAULT_LIMITS), 'keep_awake', 'host_active_cap'])
    assert.ok((PAGE_SETTINGS as readonly string[]).includes(key), key);
});
