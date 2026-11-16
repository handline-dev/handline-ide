// contract/fixtures/task_plan.json and task_review.json against the fake over Unix sockets, one for the founder and one for an agent, through the conformance
// harness every daemon runs.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import net from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { wrapSocket } from '@handline/client-node';
import { FakeDaemon } from '#src/daemon.ts';
import { serveSocket } from '#src/serve.ts';

const ROOT = join(import.meta.dirname, '..', '..', '..');

test('every case of task_plan.json and task_review.json passes on the fake', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'fk-'));
  const [sock, agentSock] = [join(dir, 's'), join(dir, 'a')];
  const daemon = new FakeDaemon();
  daemon.tasks.verbs = true; // as task_graph.json: hello lists every task verb
  const server = serveSocket(daemon, sock);
  const agent = net.createServer((s) => daemon.accept(wrapSocket(s), 'agent', 'handline.architect')); // local-ref-ok: a fixture agent
  agent.listen(agentSock);
  try {
    await Promise.all([once(server, 'listening'), once(agent, 'listening')]);
    const run = spawn(
      'python3',
      [
        join(ROOT, 'contract/conformance.py'),
        sock,
        '--as',
        `agent=${agentSock}`,
        '--allow-missing-file',
        join(ROOT, 'packages/fake-daemon/conformance-missing.txt'),
        '--var',
        'founder_key=x',
        join(ROOT, 'contract/fixtures/task_plan.json'),
        join(ROOT, 'contract/fixtures/task_review.json'),
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
    agent.close();
    rmSync(dir, { recursive: true, force: true });
  }
});
