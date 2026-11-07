// The app keeps a daemon running for this home, and the daemon outlives the app: it is started on its own (a session of its own, no pipe
// to the app, its output in daemon.log), so quitting the app leaves it and the agents it serves running. At launch the app adopts a
// daemon that answers with the app's own build, restarts one of another build once (the app was updated), and stops and replaces one
// that holds the home's lock but does not answer within the stop limit. While the app runs, a daemon that exits on its own is started
// again once; a second exit within a minute leaves it stopped, with the reason in the tray. Agents are never stopped: their terminals keep
// running.

import fs from 'node:fs';
import path from 'node:path';
import { mismatch } from '@handline/app-core';

export interface Cmd {
  file: string;
  args: string[];
  /** The PATH the daemon runs with; unset keeps the app's own. */
  path?: string;
  /** The login shell did not answer in time, so path is the app's own. */
  pathTimedOut?: boolean;
}

/** How the daemon is spawned: a session of its own, nothing on its stdin, its output in the log, so it outlives the app. */
export const detachedSpawn = (log: number) => ({ stdio: ['ignore', log, log] as ['ignore', number, number], detached: true });

/** How long a daemon takes to stop at most: daemon.StopLimit in go/internal/daemon/stop.go. */
export const STOP_LIMIT_MS = 20_000;
/** A daemon that exits again within this long of being started again is left stopped. */
export const RESTART_WINDOW_MS = 60_000;
/** How often the app looks whether its daemon still answers. */
export const WATCH_MS = 2_000;

export interface OwnerDeps {
  /** Whether something answers on the daemon's socket. */
  socketUp(): Promise<boolean>;
  /** Whether the daemon's loopback door, the one the window uses, answers; absent when the window does not use one. */
  doorUp?(): Promise<boolean>;
  /** The build the daemon on the socket says it is (its hello's sha); null when it says none or does not answer. */
  daemonBuild(): Promise<string | null>;
  /** The live pid of the daemon holding the home's lock, answering or not; null when none does. */
  holder(): Promise<number | null>;
  /** Asks that pid to stop (SIGTERM). */
  stopPid(pid: number): void;
  /** Stops what an older build left running beside its daemon (a separate gateway, a handlined holding the listen port); throws what holds
   * it, with its pid, when it cannot. */
  stopLeftovers(): Promise<void>;
  /** Starts the daemon on its own: it is not the app's child to wait on, and it keeps running when the app quits. */
  spawn(cmd: Cmd): void;
  /** A function when the command needs something looked up at spawn time; it throws the reason the daemon cannot start. */
  daemon: Cmd | (() => Promise<Cmd>);
  wait(ms: number): Promise<void>;
  now(): number;
  /** The build the app is; null (a run with no build stamp) adopts any daemon that answers. */
  build: string | null;
  /** How long a started daemon gets to answer, in polls of 250 ms. */
  polls?: number;
  /** A dev run on a home whose daemon is someone else's to run (hl dev, a prototype beside the installed app): it is adopted whatever
   * its build, and never started, stopped or replaced. */
  attachOnly?: boolean;
}

export type DaemonState = 'starting' | 'running' | 'stopped' | 'failed';

export class Owner {
  state: DaemonState = 'stopped';
  /** What the tray shows next to a failure. */
  reason = '';
  /** How the daemon in use came: adopted (it was running), started by this app, or none yet. */
  came: 'adopted' | 'started' | null = null;
  /** The command this app last started the daemon with; null before it has started one. */
  spawned: Cmd | null = null;
  private busy: Promise<void> | null = null;
  private restartedAt = -Infinity;
  private watching = false;
  private closed = false;
  private listeners: (() => void)[] = [];

  private d: OwnerDeps;
  constructor(d: OwnerDeps) {
    this.d = d;
  }

  onChange(f: () => void): void {
    this.listeners.push(f);
  }

  private set(state: DaemonState, reason = ''): void {
    this.state = state;
    this.reason = reason;
    for (const f of this.listeners) f();
  }

  /** One change at a time: a second call while one runs waits for it and does nothing more. */
  private one(f: () => Promise<void>): Promise<void> {
    this.busy ??= f().finally(() => (this.busy = null));
    return this.busy;
  }

  /** Brings the daemon up: adopts, replaces or starts it. Never throws: a failure is the state. */
  start(): Promise<void> {
    return this.one(async () => {
      if (this.state === 'running' && (await this.d.socketUp())) return;
      this.set('starting');
      try {
        await this.d.stopLeftovers();
        await this.bringUp();
      } catch (e) {
        this.set('failed', e instanceof Error ? e.message : String(e));
      }
    });
  }

  private async bringUp(): Promise<void> {
    if (this.d.attachOnly) {
      if (!(await this.d.socketUp())) throw new Error('nothing answers on this home’s daemon socket, and this run only attaches');
      if (!(await this.doorUp()))
        throw new Error('the daemon answers on its socket but not on its loopback door, and this run only attaches');
      this.came = 'adopted';
      this.set('running');
      this.watch();
      return;
    }
    let pid = await this.d.holder();
    if (!(await this.d.socketUp()) && pid !== null) {
      // A daemon that holds the lock may still be starting: it gets the stop limit to answer before it is replaced.
      for (let waited = 0; waited < STOP_LIMIT_MS && !(await this.d.socketUp()); waited += 250) await this.d.wait(250);
      pid = await this.d.holder();
    }
    if (await this.d.socketUp()) {
      const theirs = await this.d.daemonBuild();
      // One whose loopback door does not answer would leave the window loading: it is replaced like one of another build.
      if ((!this.d.build || !theirs || mismatch(theirs, this.d.build) === null) && (await this.doorUp())) {
        this.came = 'adopted';
        this.set('running');
        this.watch();
        return;
      }
    }
    if (pid !== null) await this.stopHolder(pid, 'the daemon already running on this home');
    await this.spawnUp();
  }

  private doorUp(): Promise<boolean> {
    return this.d.doorUp?.() ?? Promise.resolve(true);
  }

  private async stopHolder(pid: number, what: string): Promise<void> {
    this.d.stopPid(pid);
    if (!(await this.holderGone())) throw new Error(`${what} (pid ${pid}) did not stop within ${STOP_LIMIT_MS / 1000}s`);
  }

  private async spawnUp(cmd: Cmd | (() => Promise<Cmd>) = this.d.daemon): Promise<void> {
    this.spawned = typeof cmd === 'function' ? await cmd() : cmd;
    this.d.spawn(this.spawned);
    for (let i = 0; i < (this.d.polls ?? 120); i++) {
      if (await this.d.socketUp()) {
        this.came = 'started';
        this.set('running');
        this.watch();
        return;
      }
      await this.d.wait(250);
    }
    throw new Error('the daemon did not answer in time');
  }

  /** Starts the daemon again on this app's build (the tray's Restart daemon), or with another command (a listener that moves). */
  restart(cmd: Cmd | (() => Promise<Cmd>) = this.d.daemon): Promise<void> {
    return this.one(async () => {
      this.set('starting');
      try {
        const pid = await this.d.holder();
        if (pid !== null) await this.stopHolder(pid, 'the daemon');
        this.d.daemon = cmd;
        await this.spawnUp(cmd);
      } catch (e) {
        this.set('failed', e instanceof Error ? e.message : String(e));
      }
    });
  }

  /** The tray's Stop daemon: it stays stopped until it is started again. */
  stop(): Promise<void> {
    return this.one(async () => {
      try {
        const pid = await this.d.holder();
        if (pid !== null) await this.stopHolder(pid, 'the daemon');
        this.set('stopped');
      } catch (e) {
        this.set('failed', e instanceof Error ? e.message : String(e));
      }
    });
  }

  /** Shows a reason in the tray without touching what runs. */
  fail(reason: string): void {
    this.set('failed', reason);
  }

  /** Stops looking after the daemon (a test's end; the app looks until it quits). */
  close(): void {
    this.closed = true;
  }

  /** While the app runs: a daemon that exits on its own is started again once; a second exit within a minute leaves it stopped. */
  private watch(): void {
    if (this.watching) return;
    this.watching = true;
    void (async () => {
      while (!this.closed) {
        await this.d.wait(WATCH_MS);
        if (this.closed) return;
        if (this.state !== 'running' || this.busy || (await this.d.socketUp()) || (await this.d.holder()) !== null) continue;
        if (this.d.attachOnly) {
          this.set('stopped', 'it stopped, and this run only attaches');
          continue;
        }
        if (this.d.now() - this.restartedAt < RESTART_WINDOW_MS) {
          this.set('stopped', 'it stopped again within a minute of being started again');
          continue;
        }
        this.restartedAt = this.d.now();
        await this.one(async () => {
          this.set('starting');
          await this.spawnUp().catch((e: Error) => this.set('failed', e.message));
        });
      }
    })();
  }

  /** Whether the lock's holder is gone within the daemon's stop limit. */
  private async holderGone(): Promise<boolean> {
    for (let waited = 0; waited < STOP_LIMIT_MS; waited += 250) {
      if ((await this.d.holder()) === null) return true;
      await this.d.wait(250);
    }
    return (await this.d.holder()) === null;
  }
}

/** The claude the daemon runs: the named one (CLAUDE), else the first executable `claude` on the login shell's PATH, then on the app's own.
 * With none it is a bare `claude`: the daemon still starts and names it in daemon.status missing_tools, for the first-run screen. */
export function resolveClaude(named: string | undefined, paths: (string | undefined)[], isExec: (file: string) => boolean): string {
  if (named) return named;
  for (const dir of paths.flatMap((p) => (p ?? '').split(':')).filter(Boolean)) {
    if (isExec(`${dir}/claude`)) return `${dir}/claude`;
  }
  return 'claude';
}

/** The PATH the daemon runs with: the login shell's, then the app's own. A Dock launch has no shell dirs, and the daemon runs git and
 * tmux by name. */
export const daemonPath = (login: string | undefined, app: string | undefined): string => [login, app].filter(Boolean).join(':');

/** The daemon's command, from the environment: a checkout runs `go/handlined` (with the service's flags), a packaged app names its own
 * binary. `claude` is the one the daemon runs, resolved by the caller; `listen` and `advertise` are its network listener and the name a
 * pairing link carries; `web` is the built page it then serves on that address, set only while the person allows devices on the network. */
export function commands(
  env: (k: string) => string | undefined,
  root: string,
  listen: string,
  advertise: string | null = null,
  claude: string = env('CLAUDE') ?? 'claude',
  web: string | null = null,
): Cmd {
  // The app keeps the daemon of this home, so it may run on the real home: an app restarted from an agent shell inherits HANDLINE_HOME naming it.
  const homeArgs = [...(env('HOME') ? ['-home', env('HOME')!] : []), '-allow-real-home'];
  return {
    file: env('HANDLINED') ?? `${root}/go/handlined`,
    args: [
      ...homeArgs,
      '-claude',
      claude,
      '-terminal-tick',
      '-listen',
      listen,
      ...(advertise ? ['-advertise', advertise] : []),
      ...(web ? ['-web', web] : []),
    ],
  };
}

/** The pid the daemon wrote in this home's locks/daemon.lock, or null. The file outlives its writer: the caller checks the pid is alive. */
export function lockPid(home: string): number | null {
  try {
    const pid = parseInt(fs.readFileSync(path.join(home, 'locks', 'daemon.lock'), 'utf8').trim(), 10);
    return pid > 0 ? pid : null;
  } catch {
    return null;
  }
}

/** The pid an old separate gateway (handlined -gateway, gone since the daemon serves the listener itself) wrote in the protected store,
 * or null when it wrote none. */
export function oldGatewayPid(protectedDir: string): number | null {
  try {
    const pid = parseInt(fs.readFileSync(path.join(protectedDir, 'gateway', 'pid'), 'utf8').trim(), 10);
    return pid > 0 ? pid : null;
  } catch {
    return null;
  }
}

/** The process listening on a port, from `lsof -nP -iTCP:<port> -sTCP:LISTEN -Fpc` output (lines p<pid>, c<command>), or null. */
export function portHolder(lsof: string): { pid: number; command: string } | null {
  const pid = parseInt(/^p(\d+)$/m.exec(lsof)?.[1] ?? '', 10);
  return pid > 0 ? { pid, command: /^c(.*)$/m.exec(lsof)?.[1] ?? '?' } : null;
}
