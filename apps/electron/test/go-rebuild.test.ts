import { test } from 'node:test';
import assert from 'node:assert/strict';
import { buildsFromHere, goRebuilder, isDaemonSource, type Rebuilt } from '#src/go-rebuild.ts';

const world = (results: Rebuilt[]) => {
  const calls: string[] = [];
  const log: string[] = [];
  let release: (() => void) | undefined;
  const g = goRebuilder({
    settleMs: 5,
    log: (l) => log.push(l),
    rebuild: async () => {
      calls.push('rebuild');
      if (calls.length === 1 && results.length > 1) await new Promise<void>((r) => (release = r));
      return results.shift() ?? { ok: true, text: 'ok' };
    },
  });
  return { g, calls, log, release: () => release?.() };
};

test('only a Go source that is not a test counts', () => {
  assert.ok(isDaemonSource('scheduler/tick/tick.go'));
  assert.ok(!isDaemonSource('scheduler/tick/tick_test.go'));
  assert.ok(!isDaemonSource('kit/store/migrations/0001.sql'));
});

test('a Go save, however many files it touches, makes one rebuild and prints its line', async () => {
  const w = world([{ ok: true, text: '{"team":"t"}' }]);
  for (const f of ['a.go', 'b.go', 'c.go']) w.g.changed(f);
  w.g.changed('a_test.go');
  await w.g.idle();
  assert.deepEqual(w.calls, ['rebuild']);
  assert.deepEqual(w.log, ['go: {"team":"t"}']);
});

test('a failed build prints its error, says the old daemon keeps running, and the next save tries again', async () => {
  const w = world([
    { ok: false, text: 'x.go:3: undefined: y' },
    { ok: true, text: 'fixed' },
  ]);
  w.g.changed('x.go');
  await new Promise((r) => setTimeout(r, 30));
  w.release();
  await w.g.idle();
  assert.match(w.log[0]!, /the build failed, the old daemon keeps running:\nx\.go:3: undefined: y/);
  w.g.changed('x.go');
  await w.g.idle();
  assert.equal(w.calls.length, 2);
  assert.equal(w.log[1], 'go: fixed');
});

test('saves during a rebuild queue exactly one more, and run after it', async () => {
  const w = world([
    { ok: true, text: 'one' },
    { ok: true, text: 'two' },
  ]);
  w.g.changed('a.go');
  await new Promise((r) => setTimeout(r, 30));
  for (let n = 0; n < 4; n++) w.g.changed('a.go');
  await new Promise((r) => setTimeout(r, 30));
  assert.equal(w.calls.length, 1, 'the second waits for the first');
  w.release();
  await w.g.idle();
  assert.equal(w.calls.length, 2);
});

test("Go changes are applied only when the instance's worktree is this checkout", () => {
  assert.ok(buildsFromHere('/w/repo', '/w/repo'));
  assert.ok(buildsFromHere('/w/repo/', '/w/repo'));
  assert.ok(!buildsFromHere('/w/other', '/w/repo'));
  assert.ok(!buildsFromHere(undefined, '/w/repo'));
  assert.ok(!buildsFromHere('', '/w/repo'));
});
