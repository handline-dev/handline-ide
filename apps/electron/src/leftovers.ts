// What an older build may have left on this home, stopped before the app starts its own daemon: a separate gateway (handlined -gateway,
// gone since the daemon serves the listener itself) and a handlined holding the listen port. What cannot be stopped is named, with its pid.
import { execFile } from 'node:child_process';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { lockPid, oldGatewayPid, portHolder } from '#src/owner.ts';
import { founderKeyFile } from '#src/pair.ts';

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

/** Whether something is listening on this Unix socket, or on this loopback port. */
export const answers = (where: string | number): Promise<boolean> =>
  new Promise((resolve) => {
    const c = typeof where === 'number' ? net.createConnection(where, '127.0.0.1') : net.createConnection(where);
    c.setTimeout(3000, () => (c.destroy(), resolve(false))); // a full backlog never answers; the probe must not stay open
    c.once('connect', () => (c.destroy(), resolve(true)));
    c.once('error', () => (c.destroy(), resolve(false)));
  });

/** Stops an old separate gateway that still answers on the home's gateway.sock: it holds the port the daemon listens on. Only one that
 * answers is stopped, so a stale pid file names no process to kill. */
async function stopOldGateway(home: string): Promise<void> {
  const sock = path.join(home, 'gateway.sock');
  if (!(await answers(sock))) return;
  const dir = path.dirname(path.dirname(founderKeyFile(os.homedir(), home)));
  const pid = oldGatewayPid(dir);
  if (!pid) throw new Error(`an old gateway answers on ${sock} and ${path.join(dir, 'gateway', 'pid')} names no pid; stop it by hand`);
  process.kill(pid, 'SIGTERM');
  for (let i = 0; i < 40 && (await answers(sock)); i++) await sleep(250);
  if (await answers(sock)) throw new Error(`the old gateway (pid ${pid}) on ${sock} would not stop`);
}

/** Who listens on a port, by lsof. */
const listening = (port: string): Promise<{ pid: number; command: string } | null> =>
  new Promise((resolve) => execFile('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-Fpc'], (_e, out) => resolve(portHolder(out ?? ''))));

/** Frees the daemon's listen port from a handlined an older run left on it; anything else holding it is named, with its pid. This home's
 * own daemon (the lock's holder) is not a leftover: the app adopts it, or replaces it itself. */
async function freePort(home: string, listen: string): Promise<void> {
  const port = listen.slice(listen.lastIndexOf(':') + 1);
  const held = await listening(port);
  if (!held || held.pid === (await lockHolder(home))) return;
  if (held.command !== 'handlined')
    throw new Error(`port ${port} is held by ${held.command} (pid ${held.pid}); quit it, or choose another address`);
  process.kill(held.pid, 'SIGTERM');
  for (let i = 0; i < 40 && (await listening(port)); i++) await sleep(250);
  if (await listening(port)) throw new Error(`the handlined on port ${port} (pid ${held.pid}) would not stop`);
}

/** The command name of a live pid, or null when it is gone. */
const commandOf = (pid: number): Promise<string | null> =>
  new Promise((resolve) => execFile('ps', ['-p', String(pid), '-o', 'comm='], (e, out) => resolve(e ? null : out.trim() || null)));

/** The live handlined that holds home's lock, by the pid it wrote: a pid that is gone, or now names another program, holds nothing. */
export async function lockHolder(home: string): Promise<number | null> {
  const pid = lockPid(home);
  if (pid === null) return null;
  const command = await commandOf(pid);
  return command !== null && path.basename(command) === 'handlined' ? pid : null;
}

/** Stops what an older build left on home, and frees listen (host:port) when one is given. */
export async function stopLeftovers(home: string, listen: string | undefined): Promise<void> {
  await stopOldGateway(home);
  if (listen) await freePort(home, listen);
}
