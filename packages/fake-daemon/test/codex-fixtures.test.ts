// contract/fixtures/codex.json against the fake over a Unix socket: the codex contract an onboarding screen builds on.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { FakeDaemon } from '#src/daemon.ts';
import { serveSocket } from '#src/serve.ts';

const ROOT = join(import.meta.dirname, '..', '..', '..');

test('every case of codex.json passes on the fake', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'fk-'));
  const sock = join(dir, 's');
  const daemon = new FakeDaemon();
  daemon.tasks.verbs = true; // the hello must list the task verbs the spec has
  daemon.accounts.add({ name: 'cx', vendor: 'codex', login_state: 'needs_login' });
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
        ...['--var', 'founder_key=x', '--var', 'codex_source=~/.codex', '--var', 'codex_account=cx'],
        join(ROOT, 'contract/fixtures/codex.json'),
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
