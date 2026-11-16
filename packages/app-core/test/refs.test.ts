import { test } from 'node:test';
import assert from 'node:assert/strict';
import { hashNumbers } from '#src/refs.ts';

test('the #N numbers of a text, once each, never inside a word, an entity or a path', () => {
  assert.deepEqual(hashNumbers('see #42, #7 and #42 again; T9'), [42, 7]);
  assert.deepEqual(hashNumbers('a#5 &#38; a/#5 x.#5 ##5 #0 #12abc #1234567890'), []);
  assert.deepEqual(hashNumbers('#1 #2 #3', 2), [1, 2]);
});
