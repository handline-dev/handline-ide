import { test } from 'node:test';
import assert from 'node:assert/strict';
import { commandGist, isPersonKey, plainLine } from '#src/chat.ts';

test('a person key is a people.list key or the older human, never an agent key', () => {
  for (const k of ['p1', 'p12', 'human']) assert.equal(isPersonKey(k), true, k);
  for (const k of ['a10', 'handline.architect', 'p', 'pa1', '', null, undefined]) assert.equal(isPersonKey(k), false, String(k));
});

test('a Markdown line reads as plain words in a one-line preview', () => {
  assert.equal(plainLine('**정할 것**: A or B'), '정할 것: A or B');
  assert.equal(plainLine('# Title\n> quoted `code` and [a link](http://x)'), 'Title quoted code and a link');
  assert.equal(plainLine('- one\n- two'), 'one two');
  assert.equal(plainLine('2 * 3 = 6'), '2 * 3 = 6');
  assert.equal(plainLine('2*3 and 4*5'), '2*3 and 4*5');
  assert.equal(plainLine('run task_comment_add and `a_b_c`'), 'run task_comment_add and a_b_c');
  assert.equal(plainLine('a * b\nc * d'), 'a * b c * d');
});

test('a shell command says what it does only when its first word says so', () => {
  assert.equal(commandGist('cp /a/b /c'), 'Copies files');
  assert.equal(commandGist('cd /x && rm -rf y'), 'Deletes files');
  assert.equal(commandGist('pnpm test'), 'Runs tests');
  assert.equal(commandGist('git push origin HEAD:main'), 'Pushes commits');
  assert.equal(commandGist('python3 tool.py'), null);
  assert.equal(commandGist(null), null);
});
