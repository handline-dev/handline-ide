import { test } from 'node:test';
import assert from 'node:assert/strict';
import { letterOf } from '#src/keys.ts';

test('a letter shortcut is read by the key pressed, whatever the input types', () => {
  assert.equal(letterOf({ code: 'KeyM', key: 'ㅡ' }), 'm');
  assert.equal(letterOf({ code: 'KeyZ', key: 'Z' }), 'z');
  assert.equal(letterOf({ code: 'Digit0', key: '0' }), '0');
  assert.equal(letterOf({ key: 'J' }), 'j');
});
