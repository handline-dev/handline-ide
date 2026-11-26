// schedules.list on the fake: the seeded rows, one agent's, and contract/fixtures/schedules.json through the conformance harness.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { type RpcError } from '@handline/client';
import { FakeDaemon } from '#src/daemon.ts';
import { setup } from '#src/helpers.ts';
import { applySeeds } from '#src/seeds.ts';
import { serveSocket } from '#src/serve.ts';

const ROOT = join(import.meta.dirname, '..', '..', '..');
const ARCH = 'handline.architect'; // local-ref-ok

test('with no seed there are no schedules, and the zone is still given', async () => {
  const { client } = setup();
  await client.start();
  const got = await client.call('schedules.list', {});
  assert.deepEqual(got.schedules, []);
  assert.deepEqual(got.unreadable, []);
  assert.equal(got.zone.offset_min, -new Date().getTimezoneOffset());
  client.close();
});

test('the schedules seed: each time one ran is a message chat.locate finds in its agent chat', async () => {
  const { client, daemon } = setup();
  applySeeds(daemon, 'schedules,schedules-unreadable');
  await client.start();
  const all = await client.call('schedules.list', {});
  assert.equal(all.schedules.length, 4);
  assert.equal(all.unreadable.length, 1);
  assert.ok(
    all.schedules.some((s) => s.sentence === null) && all.schedules.some((s) => s.due) && all.schedules.some((s) => !s.fires.length),
  );
  for (const s of all.schedules) {
    for (const f of s.fires) {
      const at = await client.call('chat.locate', { request: f.request });
      assert.equal(at.agent, s.agent);
      assert.equal(at.message.from, 'schedule');
      assert.equal(at.message.body, `[schedule ${s.cron}] ${s.message}`);
      assert.equal(at.message.ref, f.ref);
    }
  }
  const mine = await client.call('schedules.list', { agent: ARCH });
  assert.deepEqual(
    mine.schedules.map((s) => s.id),
    all.schedules.filter((s) => s.agent === ARCH).map((s) => s.id),
  );
  assert.deepEqual(mine.unreadable, []);
  await assert.rejects(client.call('schedules.list', { agent: 'no.such-agent' }), (e: RpcError) => e.code === 'not_found');
  client.close();
});

test('every case of schedules.json passes on the fake', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'fk-'));
  const sock = join(dir, 's');
  const daemon = new FakeDaemon();
  daemon.tasks.verbs = true; // the hello must list the task verbs the spec has
  applySeeds(daemon, 'schedules,schedules-unreadable');
  const server = serveSocket(daemon, sock);
  try {
    await once(server, 'listening');
    const run = spawn(
      'python3',
      [
        join(ROOT, 'contract/conformance.py'),
        sock,
        '--allow-missing-file',
        join(ROOT, 'packages/fake-daemon/conformance-missing.txt'),
        '--allow-skip',
        '--var',
        'founder_key=x',
        join(ROOT, 'contract/fixtures/schedules.json'),
      ],
      { stdio: ['ignore', 'pipe', 'pipe'] },
    );
    let out = '';
    run.stdout.on('data', (d) => (out += d));
    run.stderr.on('data', (d) => (out += d));
    const [code] = await once(run, 'close');
    assert.equal(code, 0, out);
  } finally {
    server.close();
    rmSync(dir, { recursive: true, force: true });
  }
});
