// What keeps this Mac's daemon running for the app, in Electron terms: the daemon started on its own, the tray, its two questions, and
// the device token the app pairs for itself. The logic is in owner.ts, tray.ts, ask.ts and pair.ts, which a test drives; this file only
// connects them to Electron.
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { app, BrowserWindow, clipboard, dialog, ipcMain, Menu, nativeImage, nativeTheme, Tray } from 'electron';
import { type ShellDaemon } from '@handline/app-core';
import { PROTOCOL, webSocketTransport } from '@handline/client';
import { pinnedWebSocketTransport, unixSocketTransport } from '@handline/client-node';
import { CHANNELS, fromWindow } from '#src/bridge.ts';
import { Link, pairRemote, plainRemoteHost, helloOf, type OfferInput, type Remote, type Settings, type SettingsFile } from '#src/mode.ts';
import {
  commands,
  daemonEnv,
  daemonPath,
  detachedSpawn,
  markPathTimedOut,
  Owner,
  pathTimedOut,
  resolveClaude,
  type Cmd,
} from '#src/owner.ts';
import { answers, lockHolder, stopLeftovers } from '#src/leftovers.ts';
import { ensureToken, founderKeyFile, once, TokenFile, type PairDeps } from '#src/pair.ts';
import { grant } from '#src/grant.ts';
import { onTailnet } from '#src/tailnet.ts';
import { quitQuietly, type Ask } from '#src/ask.ts';
import {
  trayIcon as iconFor,
  trayMenu,
  type TrayActions,
  type TrayFollow,
  type TrayIcon,
  type TrayUpdate,
  type TrayView,
} from '#src/tray.ts';

/** The menu bar icon; a run that does not draw one gets a stand-in that draws nothing. */
function makeTray(draws: boolean): Tray {
  if (!draws) return { setImage: () => {}, on: () => {}, popUpContextMenu: () => {} } as unknown as Tray;
  const tray = new Tray(trayImage('plain'));
  tray.setToolTip('Handline');
  return tray;
}

/** The menu-bar icon. The plain one is a template image (trayTemplate@2x.png is picked up on a Retina screen), so macOS tints it for a light or a dark bar; one with a dot has a colour of its own, so it is drawn for the bar's brightness. */
function trayImage(kind: TrayIcon): Electron.NativeImage {
  const dir = path.join(import.meta.dirname, '../assets');
  if (kind === 'plain') {
    const img = nativeImage.createFromPath(path.join(dir, 'trayTemplate.png'));
    img.setTemplateImage(true);
    return img;
  }
  return nativeImage.createFromPath(path.join(dir, `tray-${kind}-${nativeTheme.shouldUseDarkColors ? 'dark' : 'light'}.png`));
}

/** How the app proves itself to its own daemon's socket. */
const localPairing = (home: string, socket: string): PairDeps => ({
  readKey: () => readFounderKey(home),
  open: () => unixSocketTransport(socket),
  name: 'Handline desktop',
});

const executable = (file: string): boolean => {
  try {
    fs.accessSync(file, fs.constants.X_OK);
    return fs.statSync(file).isFile();
  } catch {
    return false;
  }
};

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

/** Starts the daemon on its own: a session of its own (detached), nothing on its stdin and its output in daemon.log, and not waited on,
 * so it keeps running when the app quits. */
function spawnDetached(cmd: Cmd, log: number, home: string): void {
  markPathTimedOut(home, !!cmd.pathTimedOut);
  const p = spawn(cmd.file, cmd.args, { ...detachedSpawn(log), env: daemonEnv(process.env, cmd.path) });
  p.on('error', (e) => fs.writeSync(log, `${new Date().toISOString()} app: the daemon did not start: ${e.message}\n`));
  p.unref();
}

/** The build the daemon on the socket says it is, from a plain hello; null when it does not answer. */
async function daemonBuild(socket: string): Promise<string | null> {
  try {
    const r = await once(() => unixSocketTransport(socket), [['hello', { client: 'electron', protocol: PROTOCOL }]]);
    return typeof r.result?.daemon === 'string' ? r.result.daemon : null;
  } catch {
    return null;
  }
}

/** Every change of the daemon, with its reason, in daemon.log among the daemon's own lines: the tray forgets it. */
function logChanges(owner: Owner, log: number): void {
  owner.onChange(() => {
    if (owner.state === 'running' || owner.state === 'stopped' || owner.state === 'failed')
      fs.writeSync(
        log,
        `${new Date().toISOString()} app: the daemon is ${owner.state}${owner.came && owner.state === 'running' ? ` (${owner.came})` : ''}${owner.reason ? `: ${owner.reason}` : ''}\n`,
      );
  });
}

/** The founder key from the protected store, or null while the daemon has not made it. */
export function readFounderKey(home: string): string | null {
  try {
    return fs.readFileSync(founderKeyFile(os.homedir(), home), 'utf8').trim() || null;
  } catch {
    return null;
  }
}

const openRemote = (url: string, fp: string | null) => (fp ? pinnedWebSocketTransport(url, fp) : webSocketTransport(url));

const pairFrom = (i: OfferInput): Promise<Remote> => pairRemote(i, { open: openRemote, name: 'Handline desktop' });

function relaunchWith(file: SettingsFile, next: Settings): void {
  file.write(next);
  app.relaunch();
  quitQuietly(app);
}

export interface Own {
  /** The token to hand the open page if it read a stale one, '' when there is none to give, else null; marks it given. */
  freshToken(): string | null;
  /** Draws the tray menu again, after something it shows changed. */
  redraw(): void;
  /** The My turn count: the tray's dot and line. */
  setUnread(n: number): void;
  /** Agents mid-turn, for the tray's first line and the two questions; null while not known. */
  setWorking(n: number | null): void;
  working(): number | null;
  /** Brings this Mac's daemon up (or reaches the paired device's) and pairs; resolves with the address the listener ended up on. */
  ready(): Promise<string | undefined>;
  /** What the tray says about the daemon; its version is set once the shell's own connection says hello. */
  link: Link;
  /** The other device this app is paired with: where the window reaches it. */
  remote: Remote | null;
  /** The page's calls: pair with another device, or go back to this Mac's daemon; both relaunch the app. */
  pair(i: OfferInput): Promise<void>;
  useThisMac(): void;
  /** Starts this Mac's daemon again on the app's build (a checkout builds it first); rejects with the reason. */
  restartDaemon(): Promise<void>;
  /** The daemon as the tray shows it, its changes, and a Start; `canStart` is false while the app uses another device's. */
  daemon(): ShellDaemon;
  onDaemon(f: () => void): void;
  startDaemon(): void;
  /** Whether this Mac's daemon runs: quitting asks first only then. */
  running(): boolean;
}

/** The paired device's daemon: where it is, the device token, and the host named for a phone. */
async function attach(remote: Remote, link: Link): Promise<{ address: string; token: string; host: string | null }> {
  try {
    const version = await helloOf(remote, openRemote);
    link.set('connected', { address: remote.host });
    link.setVersion(version);
  } catch (e) {
    link.set('cannot-pair', { address: remote.host, reason: e instanceof Error ? e.message : String(e) });
  }
  return { address: remote.host, token: remote.token, host: plainRemoteHost(remote) };
}

/** The login shell's PATH as a daemon start asked it; timedOut when the shell did not answer in time. */
export interface LoginPath {
  path: string | undefined;
  timedOut: boolean;
}

/** The daemon's command with its claude resolved now, on the login shell's PATH: a Dock launch has no shell dirs. */
async function daemonCmd(o: {
  env: (k: string) => string | undefined;
  root: string;
  listen: string | undefined;
  advertise: string | null;
  web: string | null;
  loginPath: () => Promise<LoginPath>;
}): Promise<Cmd> {
  const { path: login, timedOut } = await o.loginPath();
  const claude = resolveClaude(o.env('CLAUDE'), [login, process.env.PATH], executable);
  return {
    ...commands(o.env, o.root, o.listen ?? '127.0.0.1:7443', o.advertise, claude, o.web),
    path: daemonPath(login, process.env.PATH),
    pathTimedOut: timedOut,
  };
}

/** Electron's message box, in front of the window when there is one. */
export const electronAsk: Ask = (o) => {
  const w = BrowserWindow.getFocusedWindow();
  return w ? dialog.showMessageBox(w, o) : dialog.showMessageBox(o);
};

/** What the tray's items do. */
function trayActions(
  o: { openWindow: () => void; openPage: (screen: 'inbox' | 'settings') => void; current: Settings; setOpenAtLogin: (on: boolean) => void },
  draw: () => void,
): TrayActions {
  return {
    open: o.openWindow,
    inbox: () => o.openPage('inbox'),
    setOpenAtLogin: (on) => {
      o.current.openAtLogin = on;
      o.setOpenAtLogin(on);
      draw();
    },
    quit: () => app.quit(),
    copy: (text) => clipboard.writeText(text),
  };
}

/** The tray's icon and menu; gives what draws the icon again. No context menu is set: a click builds the menu, and Option held at the
 * click adds the details. */
function trayControls(tray: Tray, owner: Owner, actions: TrayActions, view: () => Omit<TrayView, 'details'>): () => void {
  const draw = () => tray.setImage(trayImage(iconFor(owner.state, { ...view(), details: false })));
  const menu = (details: boolean) => Menu.buildFromTemplate(trayMenu(owner.state, actions, { ...view(), details }));
  tray.on('click', (e) => tray.popUpContextMenu(menu(!!e.altKey)));
  tray.on('right-click', (e) => tray.popUpContextMenu(menu(!!e.altKey)));
  return draw;
}

/** SIGTERM, then SIGKILL for a daemon past its stop limit. */
const signals = {
  stopPid: (pid: number) => process.kill(pid, 'SIGTERM'),
  killPid: (pid: number) => process.kill(pid, 'SIGKILL'),
};

export function ownDaemon(o: {
  env: (k: string) => string | undefined;
  home: string;
  root: string;
  socket: string;
  /** host:port of the daemon's network listener. */
  listen: string | undefined;
  /** The MagicDNS name:port a phone's pairing link carries, when the listener is on the Tailscale address. */
  advertise: string | null;
  /** The built page the daemon serves on every address it is reachable on. */
  web: string | null;
  origin: string;
  openWindow: () => void;
  /** Shows the window on a page that has no id: 'inbox' or 'settings'. */
  openPage: (screen: 'inbox' | 'settings') => void;
  settings: SettingsFile;
  current: Settings;
  /** Open at login, as the tray's checkbox changes it: kept in the settings and handed to the system. */
  setOpenAtLogin: (on: boolean) => void;
  loginPath: () => Promise<LoginPath>;
  /** Follow-main mode adds its items to the tray. */
  follow?: () => TrayFollow;
  /** The update flow, for the menu's update lines and the icon. */
  update?: () => TrayUpdate;
  /** Whether this run draws a menu bar icon (drawsTray in background.ts). */
  tray: boolean;
  /** The build the app is (its sha): a daemon of another build is started again at launch. A checkout builds the daemon before Restart
   * daemon starts it again; an installed app has no build step, so its daemon is started again from the bundle it carries. */
  build: string | null;
  buildDaemon?: () => Promise<void>;
}): Own {
  const log = fs.openSync(path.join(app.getPath('userData'), 'daemon.log'), 'a');
  const remote = o.current.remote;
  // HANDLINE_ATTACH=1: a dev run on a home whose daemon is not this app's to start or stop (hl dev, try-prototype beside the installed app).
  const attachOnly = o.env('ATTACH') === '1';
  const mine = !remote && !attachOnly;
  // The window's loopback door: the listen port on 127.0.0.1 (windowDaemon).
  const door = o.listen ? Number(o.listen.slice(o.listen.lastIndexOf(':') + 1)) : null;
  const owner = new Owner({
    socketUp: () => answers(o.socket),
    ...(door !== null && { doorUp: () => answers(door) }),
    daemonBuild: () => daemonBuild(o.socket),
    holder: () => lockHolder(o.home),
    ...signals,
    stopLeftovers: () => stopLeftovers(o.home, o.listen),
    spawn: (cmd) => spawnDetached(cmd, log, o.home),
    daemon: () => daemonCmd(o),
    wait: sleep,
    now: () => Date.now(),
    build: o.build,
    attachOnly,
  });
  const tray = makeTray(o.tray);
  const link = new Link();
  link.app = o.build;
  link.set('connecting', { address: remote?.host ?? o.listen ?? null });
  const relaunchAs = (next: Settings) => relaunchWith(o.settings, next);
  const pair = async (i: OfferInput) => relaunchAs({ ...o.current, remote: await pairFrom(i) });
  link.onChange(() => draw());
  const restartDaemon = async () => {
    if (!mine) throw new Error('this app does not run this daemon; restart it where it runs');
    await o.buildDaemon?.();
    await owner.restart(() => daemonCmd(o));
    if (owner.state !== 'running') throw new Error(owner.reason);
  };
  const counts = { working: null as number | null, unread: 0 };
  const actions = trayActions(o, () => draw());
  const draw = trayControls(tray, owner, actions, () => ({
    ...counts,
    openAtLogin: o.current.openAtLogin,
    remote: remote?.host ?? null,
    link,
    update: o.update?.(),
    follow: o.follow?.(),
  }));
  owner.onChange(draw);
  logChanges(owner, log);
  draw();
  // The key stays in this process; the page gets the device token only, from its own origin's top frame.
  const tokens = new TokenFile(path.join(app.getPath('userData'), 'device.json'));
  let token: string | null = remote ? null : (tokens.read()?.token ?? null);
  let ownHost: string | null = null;
  let settled = false; // ready() has ended: the token is what it will be
  let given: string | null | undefined; // the token the page last read; it reads once per load
  ipcMain.on(CHANNELS.token, (e) => {
    if (!fromWindow(e, o.origin)) return void (e.returnValue = null);
    given = token;
    e.returnValue = grant(token, ownHost, owner, !settled);
  });
  const pairing = localPairing(o.home, o.socket);
  return {
    // '' when neither the page nor the shell has one: the page stops waiting and shows pairing.
    freshToken: () => {
      if (given === undefined) return null;
      if (given === token) return token ? null : '';
      return token ? (given = token) : null;
    },
    redraw: draw,
    setUnread: (n) => ((counts.unread = n), draw()),
    setWorking: (n) => (counts.working = n),
    working: () => counts.working,
    link,
    remote,
    restartDaemon,
    daemon: () => ({ state: owner.state, reason: owner.reason, canStart: !remote, pathTimedOut: pathTimedOut(o.home) }),
    onDaemon: (f) => owner.onChange(f),
    startDaemon: () => void owner.start(),
    running: () => mine && owner.state === 'running',
    pair,
    useThisMac: () => relaunchAs({ ...o.current, remote: null }),
    async ready() {
      try {
        return await bringUp();
      } finally {
        settled = true;
      }
    },
  };
  async function bringUp() {
    if (remote) {
      const c = await attach(remote, link);
      token = c.token;
      ownHost = c.host;
      return c.address;
    }
    if (!o.listen) return undefined;
    await owner.start();
    ownHost = onTailnet(o.listen) ? o.listen.split(':')[0]! : null; // named even when nothing started: the page shows the reason (grant)
    if (owner.state !== 'running') return o.listen;
    token = await ensureToken(pairing, tokens, sleep).catch(() => null);
    link.set('connected', { address: o.listen });
    return o.listen;
  }
}
