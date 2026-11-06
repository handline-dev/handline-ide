import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { instanceFile, offReason, readInstance, teamEnv, type Instance } from '#src/dev-team.ts';

const script = path.resolve(import.meta.dirname, '../dev.ts');
const inst: Instance = {
  team: 'handline.pc',
  home: '/d/pc',
  branch: 'b',
  sha: 's',
  socket: '/d/pc/daemon.sock',
  listen: '127.0.0.1:7452',
};

test('the env names only the instance: the live home and listener are dropped, and the app only attaches to the instance’s daemon', () => {
  const env = teamEnv(
    { PATH: '/bin', HANDLINE_HOME: '/live', HANDLINE_SOCKET: '/live/daemon.sock', HANDLINE_WS: 'ws://x' },
    inst,
    'http://127.0.0.1:5199',
  );
  assert.deepEqual(env, {
    PATH: '/bin',
    HANDLINE_HOME: '/d/pc',
    HANDLINE_DATA: '/d/pc/app-data',
    HANDLINE_LISTEN: '127.0.0.1:7452',
    HANDLINE_LISTEN_WS: '127.0.0.1:7452',
    HANDLINE_ATTACH: '1',
    HANDLINE_DEV_URL: 'http://127.0.0.1:5199',
    HANDLINE_BACKGROUND: '0',
  });
});

test("an agent's run stays in the background, and HANDLINE_BACKGROUND decides either way", () => {
  const bg = (base: NodeJS.ProcessEnv) => teamEnv(base, inst, 'http://127.0.0.1:5199').HANDLINE_BACKGROUND;
  assert.equal(bg({ HANDLINE_AGENT: 'handline.pc' }), '1');
  assert.equal(bg({ HANDLINE_AGENT: 'handline.pc', HANDLINE_BACKGROUND: '0' }), '0');
  assert.equal(bg({ HANDLINE_BACKGROUND: '1' }), '1');
  assert.equal(bg({}), '0');
});

test('an instance is on only when its daemon answers on its socket and its listener', async () => {
  const asked: string[] = [];
  const up = (on: string[]) => async (w: string) => (asked.push(w), on.includes(w));
  assert.equal(await offReason(inst, up([inst.socket, inst.listen])), null);
  assert.match(
    (await offReason(inst, up([inst.socket])))!,
    /handline\.pc's dev instance is off \(its listener does not answer on 127\.0\.0\.1:7452\)/,
  );
  assert.match((await offReason(inst, up([inst.listen])))!, /its daemon does not answer/);
  assert.match(readInstance('/nowhere/instance.json', 'handline.pc') as string, /`hl dev on handline.pc` first/);
});

// Run asynchronously: a socket served by this process must keep accepting while the script probes it.
const run = (home: string): Promise<{ code: number; stderr: string }> =>
  new Promise((resolve) =>
    execFile(process.execPath, [script, 'handline.pc'], { env: { PATH: process.env.PATH, HOME: home } }, (err, _out, stderr) =>
      resolve({ code: err ? Number(err.code) : 0, stderr }),
    ),
  );

test('pnpm dev <team> says an instance that is off and stops, starting nothing', async () => {
  const home = fs.mkdtempSync(path.join(os.tmpdir(), 'devteam-'));
  let r = await run(home);
  assert.equal(r.code, 1);
  assert.match(r.stderr, /handline\.pc has no dev instance/);
  const dir = path.join(home, '.handline-dev', 'handline.pc');
  fs.mkdirSync(dir, { recursive: true });
  const sock = path.join(dir, 'd.sock');
  const server = net.createServer().listen(sock);
  await new Promise((r) => server.once('listening', r));
  fs.writeFileSync(instanceFile(home, 'handline.pc'), JSON.stringify({ ...inst, home: dir, socket: sock, listen: '127.0.0.1:1' }));
  r = await run(home);
  server.close();
  fs.rmSync(home, { recursive: true });
  assert.equal(r.code, 1);
  assert.match(r.stderr, /its listener does not answer on 127\.0\.0\.1:1/);
});

test('pnpm dev with no team stops with the usage line and starts nothing', async () => {
  const r: { code: number; stderr: string } = await new Promise((resolve) =>
    execFile(process.execPath, [script], { env: { PATH: process.env.PATH } }, (err, _out, stderr) =>
      resolve({ code: err ? Number(err.code) : 0, stderr }),
    ),
  );
  assert.equal(r.code, 2);
  assert.match(r.stderr, /usage: pnpm dev <team>; the live app is pnpm app/);
});

test('a dev run is never a follow-main one, even from a shell that pnpm app left its marks in', () => {
  const env = teamEnv({ HANDLINE_FOLLOW: '1', HANDLINE_FOLLOW_DIR: '/clone' }, inst, 'http://127.0.0.1:5199');
  assert.equal(env.HANDLINE_FOLLOW, undefined);
  assert.equal(env.HANDLINE_FOLLOW_DIR, undefined);
});
