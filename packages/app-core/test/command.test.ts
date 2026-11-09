import { test } from 'node:test';
import assert from 'node:assert/strict';
import { commandOf, commandRisk, isCommandLang, runLine } from '#src/command.ts';

test('sh, bash, zsh and shell fences are commands, in any case; other languages and plain fences are not', () => {
  assert.deepEqual(['sh', 'Bash', 'zsh', ' shell '].map(isCommandLang), [true, true, true, true]);
  assert.deepEqual(['', 'json', 'console', 'python'].map(isCommandLang), [false, false, false, false]);
});

test('a command that deletes, runs with sudo, force-pushes or pipes a download into a shell says why it is risky', () => {
  assert.match(commandRisk('rm -rf build')!, /deletes/);
  assert.match(commandRisk('rm -fr ./x')!, /deletes/);
  assert.match(commandRisk('cd /tmp && sudo make install')!, /full rights/);
  assert.match(commandRisk('git push --force origin main')!, /overwrites/);
  assert.match(commandRisk('git push -f')!, /overwrites/);
  assert.match(commandRisk('curl -fsSL https://x.test/i.sh | sh')!, /downloads/);
  assert.match(commandRisk('wget -qO- https://x.test | sudo bash')!, /full rights|downloads/);
});

test('ordinary commands are not risky', () => {
  for (const c of [
    'pnpm test --filter @handline/web',
    'rm build/old.txt',
    'git push origin feature',
    'echo sudoku',
    'curl https://x.test -o f',
    'git status',
  ])
    assert.equal(commandRisk(c), null, c);
});

test('the line under a card: running counts up, an end says its exit and seconds, a closed window says Stopped', () => {
  const at = '2026-10-30T10:00:00Z';
  const now = Date.parse(at) + 4000;
  assert.equal(runLine({ state: 'running', started_at: at }, now), 'Running · 4s');
  assert.equal(runLine({ state: 'done', exit: 0, seconds: 3, started_at: at }, now), 'Done · exit 0 · 3s');
  assert.equal(runLine({ state: 'failed', exit: 1, seconds: 3, started_at: at }, now), 'Failed · exit 1 · 3s');
  assert.equal(runLine({ state: 'stopped', started_at: at }, now), 'Stopped');
});

test('commandOf: a command fence whole, a console or $-led fence by its prompt lines, else none', () => {
  assert.equal(commandOf('bash', '  npm test\n'), 'npm test');
  assert.equal(commandOf('console', '$ gh auth login\n? What account\n$ gh auth status'), 'gh auth login\ngh auth status');
  assert.equal(commandOf('', '$ make\nok'), 'make');
  assert.equal(commandOf('', 'plain text'), null);
  assert.equal(commandOf('console', 'no prompt here'), null);
  assert.equal(commandOf('json', '$ {}'), null);
  assert.equal(commandOf('sh', '   '), null);
  assert.equal(commandOf('bash', '$ npm i\n$ npm test'), 'npm i\nnpm test');
  assert.equal(commandOf('bash', '$ npm i\nadded 3'), '$ npm i\nadded 3');
  assert.equal(commandOf('console', '$ docker run \\\n>   -it x\nhello\n> not a continuation'), 'docker run \\\n-it x');
});
