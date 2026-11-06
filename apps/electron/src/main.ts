import { execFile } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { app, BrowserWindow, desktopCapturer, ipcMain, Notification, session, shell, type WebContents } from 'electron';
import { DaemonClient } from '@handline/client';
import { unixSocketTransport } from '@handline/client-node';
import { drawsTray, inBackground } from '#src/background.ts';
import { isDebugWindow } from '#src/debug-window.ts';
import { setMenu } from '#src/menu.ts';
import { Desk } from '#src/desk.ts';
import { Composer, LOGIN_PROBE, parseLoginEnv, sweepTemp, type EditorEnv } from '#src/editor.ts';
import { serveCapture } from '#src/capture.ts';
import { gitView, scratch } from '#src/git-view.ts';
import { Opener } from '#src/open.ts';
import { handleOpenIn } from '#src/open-in-serve.ts';
import { Devices } from '#src/devices.ts';
import { Preview } from '#src/preview.ts';
import { followMain } from '#src/follow-app.ts';
import { keepPlace, savedFrame, savedRoute, type Frame } from '#src/window-place.ts';
import type { Follower } from '#src/follow.ts';
import { remoteUrl, SettingsFile, type Link, type Remote } from '#src/mode.ts';
import { ownDaemon, readFounderKey, type Own } from '#src/own.ts';
import { quitQuietly } from '#src/ask.ts';
import { askBeforeQuit, countWorking, loginLaunch, saveOpenAtLogin, trayFollow, trayUpdate } from '#src/app-life.ts';
import { addressesChosen, listenAddress, magicDnsName, onTailnet, TAILSCALE_CLIS, windowDaemon } from '#src/tailnet.ts';
import { Cli, Connection, DaemonView, Page, Settings, Update, CHANNELS } from '#src/bridge.ts';
import { serveDir } from '#src/serve.ts';
import { buildId, UNSENT_PROBE, Updater } from '#src/update.ts';
import { UpdateFlow } from '#src/update-flow.ts';
import { startPkgUpdate } from '#src/pkg-update-app.ts';
import { bundleAt } from '#src/bundle.ts';
import { readBuildSha } from '#src/build-id.ts';
import { removeOldService } from '#src/old-service.ts';
import { cliState, linkCli } from '#src/cli.ts';

const env = (k: string) => process.env[`HANDLINE_${k}`];
/** The same rule the daemon uses: the environment's, else ~/.handline. */
const home = env('HOME') ?? path.join(os.homedir(), '.handline');
/** The daemon's socket for this shell's own connection (badge, notices); the web app reaches the daemon by its WebSocket. */
const socket = env('SOCKET') ?? path.join(home, 'daemon.sock');
// The founder key goes only to a daemon socket inside the daemon's home: HANDLINE_SOCKET can name another process's socket.
const outsideHome = () => {
  const rel = path.relative(home, socket);
  return rel === '..' || rel.startsWith(`..${path.sep}`);
};
const founderKey = () => (outsideHome() ? null : readFounderKey(home));
/** An installed app runs the daemon it carries and serves the web build it carries, unless the environment names others. */
const carried = bundleAt(app.isPackaged, process.resourcesPath);
if (carried) process.env.HANDLINE_HANDLINED ??= carried.handlined;
/** Whether the person chose the addresses devices connect on (network_addresses in the home's config.yaml): the daemon then serves them itself. */
function chosenAddresses(): boolean {
  try {
    return addressesChosen(fs.readFileSync(path.join(home, 'config.yaml'), 'utf8'));
  } catch {
    return false;
  }
}
/** The daemon's network listener (host:port): the web app reaches it at /rpc with a device token the app got for itself. An explicit WS address with no listener is the old plain mode. */
let listen = env('WS') && !env('LISTEN') ? undefined : listenAddress(env('LISTEN'), os.networkInterfaces(), 7443, chosenAddresses());
/** The checkout the app runs from; a packaged app names its binaries with HANDLINE_HANDLINED. */
const root = path.resolve(import.meta.dirname, '../../..');
/** The window always carries its daemon: a built page without `?daemon=` would take its own origin, which here is the static server. */
let ws = env('WS') ?? (listen ? windowDaemon(listen) : 'ws://127.0.0.1:7777');
app.setName('Handline'); // the menu bar and the About panel; the bundle name of an unpackaged run is set by launch.ts
const APP_ICON = path.join(import.meta.dirname, '../assets/icon.png');
const dist = env('WEB_DIST') ?? carried?.web ?? path.resolve(import.meta.dirname, '../../web/dist');

let win: BrowserWindow | null = null;
/** Where an update is, for the notice, the tray and both sources of updates. */
const flow = new UpdateFlow();

/** VISUAL, EDITOR and PATH: the shell's own environment, else the login shell's, asked once (a Dock launch has neither). */
let login: Promise<EditorEnv> | null = null;
async function editorEnv(): Promise<EditorEnv> {
  if (process.env.VISUAL || process.env.EDITOR) return { VISUAL: process.env.VISUAL, EDITOR: process.env.EDITOR, PATH: process.env.PATH };
  login ??= new Promise<EditorEnv>((resolve) => {
    execFile(process.env.SHELL ?? '/bin/sh', ['-ilc', LOGIN_PROBE], { timeout: 3000, encoding: 'utf8' }, (err, out) =>
      resolve(err && !out ? {} : parseLoginEnv(out)),
    );
  });
  return login;
}

/** The listener takes a browser's socket only from its own origin; this page is served here, so the shell says so for that socket. The device token still decides. */
function allowListenerOrigin(): void {
  session.defaultSession.webRequest.onBeforeSendHeaders({ urls: ['ws://*/*', 'wss://*/*'] }, (d, done) => {
    const host = new URL(d.url).host;
    const ours = listen && (host === listen || host === new URL(ws).host);
    done({ requestHeaders: ours ? { ...d.requestHeaders, Origin: `http://${host}` } : d.requestHeaders });
  });
}

const background = inBackground(process.env);
/** The build this app is, read once at launch: an update builds a newer one on disk while this one still runs. */
const runningBuild = readBuildSha(import.meta.dirname);

/** Brings the window to the front, except in a background run, whose window stays hidden (the DevTools port still draws and captures it). */
function show(): void {
  if (!win || win.isDestroyed() || background) return;
  if (win.isMinimized()) win.restore();
  win.show();
  win.focus();
}

/** HANDLINE_DEV_URL: a vite dev server stands in for the built dist, so the page hot-reloads as the sources change (a dev aid). */
async function pageServer(): Promise<{ url: string; close: () => void }> {
  const devUrl = env('DEV_URL');
  return devUrl ? { url: devUrl, close: () => undefined } : serveDir(dist, Number(env('WEB_PORT') ?? (listen ? 17443 : 17444)));
}

/** `name.ts.net:port` for the listener when Tailscale is up and names this machine, else null (the pairing link then carries the address). */
async function advertised(listen: string): Promise<string | null> {
  for (const cli of TAILSCALE_CLIS) {
    const out = await new Promise<string>((resolve) =>
      execFile(cli, ['status', '--json'], { timeout: 5000, encoding: 'utf8' }, (err, o) => resolve(err ? '' : o)),
    );
    const name = magicDnsName(out);
    if (name) return `${name}:${listen.split(':')[1] ?? '7443'}`;
  }
  return null;
}

/** A paired remote daemon has a self-signed certificate: the page trusts it by the fingerprint pairing handed over, and no other. */
function trustFingerprint(remote: Remote): void {
  app.on('certificate-error', (e, _wc, url, _err, cert, trust) => {
    e.preventDefault();
    const got = Buffer.from(cert.fingerprint.replace(/^sha256\//, ''), 'base64').toString('hex');
    trust(!!remote.fingerprint && new URL(url).host === remote.host && got === remote.fingerprint);
  });
}

/** The window connects as a paired device, which may not read or write settings; the shell's founder connection does it for the page. */
function handleSettings(origin: string, client: DaemonClient, remote: Remote | null): void {
  const otherDevice = new Error('This app is paired with another device; change its settings there.');
  const s = new Settings(origin, {
    get: (agent) => (remote ? Promise.reject(otherDevice) : client.call('settings.get', { agent })),
    set: (changes, agent) => (remote ? Promise.reject(otherDevice) : client.call('settings.set', { changes, agent })),
  });
  ipcMain.handle(CHANNELS.settingsGet, (e, arg) => s.get(e, arg));
  ipcMain.handle(CHANNELS.settingsSet, (e, arg) => s.set(e, arg));
}

function handleConnection(origin: string, own: Own, remote: Remote | null, f?: Follower): void {
  const c = new Connection(origin, {
    // Only a daemon this app started, and only when the build on disk is this one: a newer one (main moved) is the follow offer's to apply.
    build: () => ({ sha: runningBuild, canRestart: !remote && readBuildSha(import.meta.dirname) === runningBuild }),
    restartDaemon: () => own.restartDaemon(),
    // The page's words for which daemon this is: this Mac's (own), or the paired device's.
    mode: () => ({ mode: remote ? 'connect' : 'own', remote: remote?.host ?? null }),
    follow: () => (f ? { sha: f.sha, state: f.state, next: f.next?.sha ?? null } : null),
    check: async () => void (await f?.look()),
    // Going back to this Mac's daemon is the one switch left; another device's is reached by pairing.
    setMode: (m) => m === 'own' && remote && own.useThisMac(),
    pair: (i) => own.pair(i),
  });
  ipcMain.handle(CHANNELS.mode, (e) => c.mode(e));
  ipcMain.handle(CHANNELS.setMode, (e, arg) => c.setMode(e, arg));
  ipcMain.handle(CHANNELS.build, (e) => c.build(e));
  ipcMain.handle(CHANNELS.restartDaemon, (e) => c.restartDaemon(e));
  const d = new DaemonView(origin, { view: () => own.daemon(), start: () => own.startDaemon() });
  ipcMain.handle(CHANNELS.daemon, (e) => d.view(e));
  ipcMain.handle(CHANNELS.startDaemon, (e) => d.start(e));
  own.onDaemon(() => win?.webContents.send(CHANNELS.daemon, own.daemon()));
  ipcMain.handle(CHANNELS.follow, (e) => c.follow(e));
  ipcMain.handle(CHANNELS.checkUpdate, (e) => c.checkUpdate(e));
  ipcMain.handle(CHANNELS.pairRemote, (e, arg) => c.pairRemote(e, arg));
  serveCapture(ipcMain, origin, { sources: (thumbnailSize) => desktopCapturer.getSources({ types: ['window'], thumbnailSize }) });
}

/** The Settings row that links `hl` into ~/.local/bin, to the one inside the bundle: when the app moves, the row shows it unlinked and links it again. */
function handleCli(origin: string): void {
  const cliDir = path.join(os.homedir(), '.local', 'bin');
  const hl = carried?.hl ?? null;
  const cli = new Cli(origin, { state: () => cliState(hl, cliDir), link: () => hl && linkCli(hl, cliDir) });
  ipcMain.handle(CHANNELS.cli, (e) => cli.state(e));
  ipcMain.handle(CHANNELS.linkCli, (e) => cli.link(e));
}

function handleDevices(origin: string): void {
  const devices = new Devices(origin);
  ipcMain.handle(CHANNELS.deviceShot, (e, arg) => devices.shot(e, arg));
  ipcMain.handle(CHANNELS.deviceTap, (e, arg) => devices.tap(e, arg));
  ipcMain.handle(CHANNELS.deviceList, (e) => devices.list(e));
  ipcMain.handle(CHANNELS.deviceSwipe, (e, arg) => devices.swipe(e, arg));
  ipcMain.handle(CHANNELS.deviceText, (e, arg) => devices.text(e, arg));
}

/** The shell's own connection to the daemon. */
const shellClient = (remote: Remote | null): DaemonClient =>
  new DaemonClient({ client: 'electron', connect: () => unixSocketTransport(socket), key: founderKey, founder: !remote && !outsideHome() });

/** On every connect, a daemon restarted by an update included, the tray learns which build it now is. */
function watchVersion(client: DaemonClient, link: Link): void {
  link.setVersion(client.hello?.daemon);
  client.onState((st) => st === 'open' && link.setVersion(client.hello?.daemon));
}

/** Runs `f` until it stops throwing, two seconds apart. */
async function untilUp(f: () => Promise<unknown>): Promise<void> {
  for (;;) {
    try {
      return void (await f());
    } catch {
      await new Promise((r) => setTimeout(r, 2000));
    }
  }
}

/** A paired remote: its daemon is the one the page talks to, and its certificate the one trusted. */
function useRemote(remote: Remote): void {
  listen = remote.host;
  ws = remoteUrl(remote);
  trustFingerprint(remote);
}

/** Builds a checkout's go/handlined, the daemon this app starts. */
const buildDaemon = (): Promise<void> =>
  new Promise((resolve, reject) =>
    execFile('bash', ['tools/build-go.sh'], { cwd: root }, (err, _out, errOut) =>
      err ? reject(new Error(`build-go.sh: ${(errOut || err.message).trim().split('\n').pop()}`)) : resolve(),
    ),
  );

/** True when nothing on screen would be lost by an update's question. The page says when it holds something unsent; until it has said so once, the shell looks into the page itself. */
const idleOf = (page: Page, composer: Composer) => async (): Promise<boolean> => {
  if (background || !win || win.isDestroyed() || composer.running) return false; // an open editor counts as unsent
  if (page.unsent !== null) return !page.unsent;
  return !(await win.webContents.executeJavaScript(UNSENT_PROBE).catch(() => true));
};

function makeUpdater(page: Page, composer: Composer, flow: UpdateFlow): Updater {
  return new Updater(
    {
      build: () => buildId({ page: path.join(dist, 'index.html'), main: import.meta.filename }),
      idle: idleOf(page, composer),
      // The daemon keeps running; the relaunched app adopts it, or starts it again when it is of another build.
      restart: async () => {
        app.relaunch();
        quitQuietly(app);
      },
    },
    flow,
  );
}

/** The update notice's calls: its state, and restart, later, try again or the log of a failure. */
function handleUpdate(origin: string, flow: UpdateFlow, redraw: () => void): void {
  const log = path.join(app.getPath('userData'), 'update.log');
  const u = new Update(origin, {
    view: () => flow.view,
    act: (what) => flow.act(what),
    details: () => {
      fs.writeFileSync(log, flow.log || flow.view.reason, { mode: 0o600 });
      void shell.openPath(log);
    },
  });
  ipcMain.handle(CHANNELS.update, (e) => u.view(e));
  ipcMain.handle(CHANNELS.updateAct, (e, arg) => u.act(e, arg));
  flow.onChange(() => {
    redraw();
    win?.webContents.send(CHANNELS.update, flow.view);
  });
}

/** The calls the shell makes on its own founder connection for the page: settings, and openIn. */
function handleFounderCalls(origin: string, client: DaemonClient, remote: Remote | null, tmp: string): void {
  handleSettings(origin, client, remote);
  handleOpenIn(origin, client, tmp);
}

/** The app window: the page gets window.handline and nothing else, and the window never leaves the loopback origin. */
function createWindow(origin: string, onReady: () => void, frame: Frame | null): BrowserWindow {
  const w = new BrowserWindow({
    ...(frame ? { x: frame.x, y: frame.y } : {}),
    width: frame?.width ?? 1280,
    height: frame?.height ?? 860,
    title: 'Handline',
    icon: APP_ICON,
    show: false,
    // No OS title bar; the traffic lights sit over the sidebar (styles/base.css makes room).
    ...(os.platform() === 'darwin' ? { titleBarStyle: 'hiddenInset' as const, trafficLightPosition: { x: 14, y: 15 } } : {}),
    // The page gets window.handline from the preload and nothing else (docs/electron-bridge.md §2).
    webPreferences: {
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      preload: path.join(import.meta.dirname, 'preload.cjs'),
      // A hidden window would be throttled; a background run's page must keep drawing for its checks and captures.
      backgroundThrottling: !background,
    },
  });
  // The traffic lights are gone in full screen: the page drops the room it keeps for them (shell.ts).
  const tellFull = () => w.webContents.send(CHANNELS.fullScreen, w.isFullScreen());
  w.on('enter-full-screen', tellFull);
  w.on('leave-full-screen', tellFull);
  w.webContents.on('did-finish-load', tellFull);
  w.once('ready-to-show', onReady);
  if (frame?.maximized) w.maximize();
  if (frame?.fullScreen) w.setFullScreen(true);
  confine(w.webContents, origin);
  return w;
}

/**
 * The window shows the app and nothing else: a link goes to the browser (http/https only), and the window never leaves the loopback
 * origin. The one window the page may open is the debug window (Settings' bottom button), on the same origin and with the same
 * preferences, and it is confined the same way.
 */
function confine(contents: WebContents, origin: string): void {
  contents.setWindowOpenHandler(({ url }) => {
    if (isDebugWindow(url, origin))
      return { action: 'allow', overrideBrowserWindowOptions: { width: 1280, height: 820, title: 'Terminals (debug)' } };
    if (/^https?:\/\//.test(url)) void shell.openExternal(url);
    return { action: 'deny' };
  });
  contents.on('did-create-window', (child) => confine(child.webContents, origin));
  contents.on('will-navigate', (e, url) => {
    if (new URL(url).origin !== origin) e.preventDefault();
  });
}

// A port for DevTools protocol checks of a run (HANDLINE_DEBUG_PORT=9333).
if (env('DEBUG_PORT')) app.commandLine.appendSwitch('remote-debugging-port', env('DEBUG_PORT')!);

// HANDLINE_DATA: a data folder of one's own - a dev instance then keeps its settings, its device
// token and its single-instance lock apart from the everyday app and runs next to it (a dev aid).
const data = env('DATA');
if (data) app.setPath('userData', data);

if (!app.requestSingleInstanceLock({ background })) {
  // The first instance gets 'second-instance' and focuses its window, unless this run is a background one. This run says why it quits:
  // the running instance may be an older build, which keeps serving until it is quit.
  console.error(
    'Handline is already running, so this run quits and the running app keeps serving. To run this build, quit Handline first.',
  );
  app.quit();
} else {
  app.on('second-instance', (_e, _argv, _cwd, data) => (data as { background?: boolean } | undefined)?.background || show());
  if (background) app.setActivationPolicy('accessory'); // no dock icon, and the app never becomes the active one
  void app.whenReady().then(async () => {
    // An unpackaged app shows Electron's icon in the dock until it is told otherwise.
    app.dock?.setIcon(APP_ICON);
    allowListenerOrigin();
    const settings = new SettingsFile(path.join(app.getPath('userData'), 'settings.json'));
    // This Mac's daemon, unless the app was paired with another device's.
    const current = settings.read();
    const remote = current.remote;
    const login = loginLaunch(current.openAtLogin, background);
    if (remote) useRemote(remote);
    // The device token, the layouts and the settings are kept per page origin, so the page keeps one port from launch to launch.
    const web = await pageServer();
    const origin = new URL(web.url).origin;
    const page = new Page(origin);
    // The shell's own calls (an agent's worktree, the badge) are the founder's: hello carries the key from the protected store, as pairing does.
    const client = shellClient(remote);
    // Follow-main mode (pnpm app): the app runs from its own checkout of main and offers to restart when main moves.
    const following = env('FOLLOW') ? followMain({ dir: root, window: () => win, changed: () => own.redraw(), flow }) : null;
    const f = following?.follower;
    const own: Own = ownDaemon({
      env,
      loginPath: async () => (await editorEnv()).PATH,
      home,
      root,
      tray: drawsTray(process.env, app.isPackaged),
      socket,
      listen,
      advertise: listen && onTailnet(listen) ? await advertised(listen) : null,
      web: dist,
      origin,
      settings,
      current,
      setOpenAtLogin: saveOpenAtLogin(settings),
      build: runningBuild,
      buildDaemon: env('HANDLINED') ? undefined : buildDaemon, // a packaged app names its bundle's handlined
      openWindow: () => void openWindow(),
      openPage: (screen) => void openWindow().then(() => win?.webContents.send(CHANNELS.navigate, { screen, id: '', event: 'tray' })),
      update: () => trayUpdate(flow),
      follow: f && (() => trayFollow(f)),
    });
    handleConnection(origin, own, remote, f);
    handleCli(origin);
    ipcMain.handle(CHANNELS.screen, (e, arg) => page.screen(e, arg));
    ipcMain.handle(CHANNELS.unsent, (e, arg) => page.setUnsent(e, arg));
    const preview = new Preview(origin, () => win);
    ipcMain.handle(CHANNELS.preview, (e, arg) => preview.set(e, arg));
    ipcMain.handle(CHANNELS.capturePreview, (e) => preview.capture(e));
    handleDevices(origin);
    handleFounderCalls(origin, client, remote, app.getPath('temp'));
    // The worktree of an agent comes from the shell's own daemon connection, never from the page.
    const opener = new Opener(origin, {
      worktreeOf: async (agent) => (await client.call('agent.get', { agent })).worktree?.path ?? null,
      open: (p) => shell.openPath(p),
      reveal: (p) => shell.showItemInFolder(p),
      git: gitView,
      openExternal: (url) => shell.openExternal(url),
      scratch: (name, bytes) => scratch(app.getPath('temp'), name, bytes),
    });
    const composer = new Composer(origin, { env: editorEnv, tmp: app.getPath('temp') });
    ipcMain.handle(CHANNELS.openPath, (e, arg) => opener.openPath(e, arg));
    ipcMain.handle(CHANNELS.compose, (e, arg) => composer.compose(e, arg));
    ipcMain.handle(CHANNELS.cancelCompose, (e, arg) => composer.cancel(e, arg));
    app.on('quit', () => composer.dispose());
    void sweepTemp(app.getPath('temp'));
    const openWindow = async (): Promise<void> => {
      login.shown();
      if (win && !win.isDestroyed()) return show();
      const frame = background ? null : savedFrame();
      win = createWindow(origin, show, frame);
      if (!background) keepPlace(win);
      await win.loadURL(`${web.url}?daemon=${encodeURIComponent(ws)}${background ? '' : savedRoute()}`);
    };
    setMenu(openWindow, () => win?.webContents);
    // The app lives in the tray: closing the last window leaves it running.
    app.on('activate', () => void openWindow());
    app.on('window-all-closed', () => undefined);
    app.on('quit', () => web.close());

    handleUpdate(origin, flow, () => own.redraw());
    // The launchd job an older Handline installed would start a daemon under this app's own: it goes before the app starts its daemon.
    if (!remote && !env('HOME'))
      await removeOldService().catch((e: Error) => console.warn(`the old daemon service was not removed: ${e.message}`));
    const updater = makeUpdater(page, composer, flow);
    askBeforeQuit(own, settings);
    // The window opens at once and shows the daemon starting; the page reconnects once it answers. At login there is no window.
    const opening = login.hidden() || background ? Promise.resolve() : openWindow();
    void own
      .ready()
      .then(async () => {
        const fresh = own.freshToken();
        if (fresh && win && !win.isDestroyed()) win.webContents.send(CHANNELS.deviceToken, fresh);
      })
      .then(() => opening)
      .then(() => (background ? openWindow() : undefined))
      .then(() => following?.start());
    const checking = setInterval(() => void updater.check().catch(() => undefined), 30_000);
    app.on('quit', () => clearInterval(checking));
    app.on('quit', startPkgUpdate({ flow, idle: idleOf(page, composer) }));

    const desk = new Desk(client, {
      setBadge: (t, n) => {
        own.setUnread(n);
        if (app.dock) app.dock.setBadge(t);
        else app.setBadgeCount(n);
      },
      notify: (n, onClick) => {
        if (background) return;
        const notice = new Notification({ title: n.title, body: n.body });
        notice.on('click', onClick);
        notice.show();
      },
      screen: () => ({ focused: !!win?.isFocused(), agent: page.agent }),
      open: (n) => {
        show();
        if (n.target && win) win.webContents.send(CHANNELS.navigate, { screen: n.target.screen, id: n.target.id, event: n.event });
      },
    });
    // The client reconnects once it has been up; a daemon that is down at launch is retried here.
    void (async () => {
      await untilUp(() => client.start());
      if (!remote) watchVersion(client, own.link);
      await untilUp(() => desk.start());
    })();
    countWorking(client, own);
  });
}
