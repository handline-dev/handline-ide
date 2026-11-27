import { test } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { Linter } from 'eslint';
import tseslint from 'typescript-eslint';
import { copyRule, isSentence } from '#eslint/copy.js';

const file = path.resolve(import.meta.dirname, '../../../apps/web/src/zz.tsx');

function lint(code: string, baseline: Record<string, string[]> = {}): string[] {
  const linter = new Linter({ cwd: path.resolve(import.meta.dirname, '../../..') });
  const messages = linter.verify(
    code,
    [
      {
        files: ['**/*.tsx'],
        languageOptions: { parser: tseslint.parser, parserOptions: { ecmaFeatures: { jsx: true } } },
        plugins: { handline: { rules: { copy: copyRule } } },
        rules: { 'handline/copy': ['error', { baseline }] },
      },
    ],
    { filename: file },
  );
  return messages.map((m) => m.messageId ?? m.message);
}

test('a sentence fails', () => {
  assert.deepEqual(lint('const a = <p>Nothing here yet.</p>;'), ['sentence']);
  assert.deepEqual(lint('const a = <p>Waiting for the machine to calm down first</p>;'), ['sentence']);
  assert.deepEqual(lint('const a = <input placeholder="Say what the agent should do." />;'), ['sentence']);
  assert.deepEqual(lint('const a = <b title={`Start it now.`} />;'), ['sentence']);
});

test('a label passes', () => {
  assert.deepEqual(lint('const a = <button title="Start now">Waiting · machine busy</button>;'), []);
  assert.deepEqual(lint('const a = <p>v1.2.3</p>;'), []);
});

test('a baseline entry passes once per listing, a new sentence fails', () => {
  const baseline = { 'apps/web/src/zz.tsx': ['No changes.'] };
  assert.deepEqual(lint('const a = <p>No changes.</p>;', baseline), []);
  assert.deepEqual(lint('const a = <><p>No changes.</p><p>No changes.</p></>;', baseline), ['sentence']);
  assert.deepEqual(lint('const a = <p>Nothing here.</p>;', baseline), ['stale', 'sentence']);
});

test('a baseline entry that is gone fails, so the list only shrinks', () => {
  assert.deepEqual(lint('const a = <p>Changes</p>;', { 'apps/web/src/zz.tsx': ['No changes.'] }), ['stale']);
});

test('a copy-ok line is allowed', () => {
  assert.deepEqual(lint('const a = <p>{/* copy-ok */}A sentence the page needs.</p>;'), []);
});

test('the sentence test', () => {
  assert.equal(isSentence('Start now'), false);
  assert.equal(isSentence('…'), false);
  assert.equal(isSentence('Done.'), true);
  assert.equal(isSentence('one two three four five six seven'), true);
});
