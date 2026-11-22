// window.handline as the page sees it, made from the preload's ipcRenderer (docs/electron-bridge.md §2). Calls return a result and never throw.
import type { NavigateTo, ShellApi, ShellCluster, ShellDaemon, ShellUpdate } from '@handline/app-core';
import { CHANNELS } from '#src/bridge.ts';

export interface Ipc {
  invoke(channel: string, arg: unknown): Promise<unknown>;
  on(channel: string, f: (event: unknown, arg: unknown) => void): unknown;
}

export function makeApi(ipc: Ipc, zoom: () => number = () => 1): ShellApi {
  // A click before the page registered is kept and delivered to the first listener.
  let waiting: NavigateTo | null = null;
  let listener: ((to: NavigateTo) => void) | null = null;
  ipc.on(CHANNELS.navigate, (_e, to) => {
    if (listener) listener(to as NavigateTo);
    else waiting = to as NavigateTo;
  });
  // The shell sends the state on every load and change; the last one is kept for the first listener.
  let full = false;
  const fullListeners = new Set<(full: boolean) => void>();
  ipc.on(CHANNELS.fullScreen, (_e, on) => {
    full = on === true;
    fullListeners.forEach((f) => f(full));
  });
  // The last word on the token is kept for a listener that registers later (the page may still be loading when it comes).
  let lastToken: string | null = null;
  const tokenListeners = new Set<(token: string) => void>();
  ipc.on(CHANNELS.deviceToken, (_e, t) => {
    if (typeof t !== 'string') return;
    lastToken = t;
    tokenListeners.forEach((f) => f(t));
  });
  const call = async <R>(channel: string, arg?: unknown): Promise<R> => {
    try {
      return (await ipc.invoke(channel, arg)) as R;
    } catch (e) {
      return { ok: false, code: 'bad_request', message: e instanceof Error ? e.message : String(e) } as R;
    }
  }; // The shell sends the update state, the daemon's and the clusters' on every change; the last one is kept for a later listener.
  const update = kept<ShellUpdate>(ipc, CHANNELS.update);
  const daemon = kept<ShellDaemon>(ipc, CHANNELS.daemon, async () => {
    const v = await call<{ ok: boolean } & ShellDaemon>(CHANNELS.daemon);
    return v.ok ? v : null;
  });
  const clusters = kept<ShellCluster[]>(ipc, CHANNELS.clusters, async () => {
    const v = await call<{ ok: boolean; clusters: ShellCluster[] }>(CHANNELS.clusters);
    return v.ok ? v.clusters : null;
  });

  return {
    api: 1,
    zoom,
    screen: (s) => call(CHANNELS.screen, s),
    unsent: (on) => call(CHANNELS.unsent, on),
    openPath: (a) => call(CHANNELS.openPath, a),
    openIn: (a) => call(CHANNELS.openIn, a),
    openInApps: () => call(CHANNELS.openInApps),
    installTools: (a) => call(CHANNELS.installTools, a ?? {}),
    compose: (a) => call(CHANNELS.compose, a ?? {}),
    cancelCompose: () => call(CHANNELS.cancelCompose),
    captureWindows: () => call(CHANNELS.captureWindows),
    pickFolder: () => call(CHANNELS.pickFolder),
    captureWindow: (a) => call(CHANNELS.captureWindow, a),
    webview: true,
    capturePreview: (id) => call(CHANNELS.capturePreview, id),
    deviceShot: (a) => call(CHANNELS.deviceShot, a),
    deviceTap: (a) => call(CHANNELS.deviceTap, a),
    deviceList: () => call(CHANNELS.deviceList),
    deviceSwipe: (a) => call(CHANNELS.deviceSwipe, a),
    deviceText: (a) => call(CHANNELS.deviceText, a),
    follow: () => call(CHANNELS.follow),
    checkUpdate: () => call(CHANNELS.checkUpdate),
    build: () => call(CHANNELS.build),
    restartDaemon: () => call(CHANNELS.restartDaemon),
    update: () => call(CHANNELS.update),
    updateChannel: () => call(CHANNELS.updateChannel),
    setUpdateChannel: (c) => call(CHANNELS.setUpdateChannel, c),
    updateAct: (what) => call(CHANNELS.updateAct, what),
    onUpdate: update.on,
    daemon: () => call(CHANNELS.daemon),
    startDaemon: () => call(CHANNELS.startDaemon),
    stopDaemon: () => call(CHANNELS.stopDaemon),
    promote: () => call(CHANNELS.promote),
    keepDoors: (doors) => call(CHANNELS.keepDoors, doors),
    onDaemon: daemon.on,
    cli: () => call(CHANNELS.cli),
    linkCli: () => call(CHANNELS.linkCli),
    clusters: () => call(CHANNELS.clusters),
    joinCluster: (a) => call(CHANNELS.joinCluster, a),
    leaveCluster: (id) => call(CHANNELS.leaveCluster, id),
    onClusters: clusters.on,
    onToken(cb) {
      tokenListeners.add(cb);
      if (lastToken !== null) cb(lastToken);
      return () => void tokenListeners.delete(cb);
    },
    onFullScreen(cb) {
      fullListeners.add(cb);
      cb(full);
      return () => void fullListeners.delete(cb);
    },
    settingsGet: (a) => call(CHANNELS.settingsGet, a ?? {}),
    settingsSet: (a) => call(CHANNELS.settingsSet, a),
    onNavigate(cb) {
      listener = cb;
      if (waiting) {
        const to = waiting;
        waiting = null;
        cb(to);
      }
      return () => {
        if (listener === cb) listener = null;
      };
    },
  };
}

/** The last value the shell sent on `channel`, for a listener that registers later; with none yet, `ask` fetches one for it. */
function kept<T>(ipc: Ipc, channel: string, ask?: () => Promise<T | null>) {
  let last: T | null = null;
  const listeners = new Set<(v: T) => void>();
  ipc.on(channel, (_e, v) => {
    last = v as T;
    listeners.forEach((f) => f(last!));
  });
  return {
    on(cb: (v: T) => void): () => void {
      listeners.add(cb);
      if (last) cb(last);
      else
        void ask?.().then((v) => {
          if (v && !last && listeners.has(cb)) cb(v);
        });
      return () => void listeners.delete(cb);
    },
  };
}
