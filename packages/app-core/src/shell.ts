// The surface between the web app and the desktop shell (docs/electron-bridge.md): its types and the settings keys the page may set.
// Pure, no DOM: the shell's preload and the web app both import it.

import type { Methods, SettingsChanges } from '@handline/client';
import type { PickedElement } from '#src/element-pick.ts';

/** The settings the Settings page and the status bar's resource area edit, the only keys the shell sets on its behalf (the other daemon settings are not the page's to change). */
export const PAGE_SETTINGS = [
  'park_timeout_s',
  'handoff_note',
  'handoff_prompt',
  'ignore_cache_expiry',
  'notice_digest_s',
  'bg_notice_s',
  'bg_grace_s',
  'bg_check_s',
  'push_prompt_asked',
  'push_task_done',
  'push_stall',
  'push_account_out',
  'push_only_away',
  'push_sound',
  // the status bar's resource area
  'host_active_cap',
  'keep_awake',
  'load_per_core_pct',
  'ram_free_min_pct',
] as const satisfies readonly (keyof SettingsChanges)[];
export type PageSettings = Pick<SettingsChanges, (typeof PAGE_SETTINGS)[number]>;

/** The release channels of the Mac app. */
export const UPDATE_CHANNELS = ['stable', 'nightly', 'canary'] as const;
export type UpdateChannel = (typeof UPDATE_CHANNELS)[number];

/** What every call returns when it does not succeed: calls never throw across the window's edge. */
export type Fail<C extends string> = { ok: false; code: C; message: string };

/** A clicked notice: the notifications event's type and the `target` the daemon put on it. */
export interface NavigateTo {
  screen: string;
  id: string;
  event: string;
}

/** Where an update is: preparing (with its step and how far along), ready, put off until later, or failed (with the reason). */
export interface ShellUpdate {
  phase: 'idle' | 'preparing' | 'ready' | 'later' | 'failed';
  step: string;
  fraction: number;
  reason: string;
}

/** One cluster this Mac joined: its name (the master's, else its address), its master's WebSocket and this Mac's device token there,
 * whether the master answers now, and whether this Mac is that master (promoteCluster). */
export interface ShellCluster {
  id: string;
  name: string;
  daemon: string;
  token: string;
  reachable: boolean;
  master: boolean;
}

/** This Mac's daemon, as the tray shows it: starting (also while it restarts), running, stopped, or failed with the reason. `canStart` is
 * whether the page may offer Start. */
export interface ShellDaemon {
  state: 'starting' | 'running' | 'stopped' | 'failed';
  reason: string;
  canStart: boolean;
  /** True when the login shell did not answer in time at the daemon's last start, so it runs on the app's own PATH: its missing_tools
   * may name tools that are installed. A restart asks the shell again. Absent from an app that does not say. */
  pathTimedOut?: boolean;
}

export interface ShellApi {
  /** Present when the shell is there; a change that breaks a call makes it 2. */
  api: 1;
  /** The agent whose workspace is on screen, or null. */
  screen(s: { agent: string | null }): Promise<{ ok: true } | Fail<'bad_request'>>;
  /** True while a restart would lose something. */
  unsent(on: boolean): Promise<{ ok: true } | Fail<'bad_request'>>;
  /** Open a path a message holds in the default app, or reveal it when it is something that runs. Absent in a shell that lacks it. */
  openPath?(a: {
    path: string;
    agent?: string;
    /** Branch names the message holds: a repo path opens the version on one of them (read-only), else the worktree's file, else main, else GitHub. */
    branches?: string[];
  }): Promise<{ ok: true; how: 'opened' | 'revealed' | 'branch' | 'main' | 'github' } | Fail<'bad_request' | 'not_found' | 'unavailable'>>;
  /** The agent's folder in VS Code or Terminal, or its Claude Code session in Terminal. The shell resolves the folder and the command itself; macOS only. Absent in a shell that lacks it. */
  openIn?(a: {
    app: 'vscode' | 'terminal' | 'claude';
    agent: string;
  }): Promise<{ ok: true } | Fail<'bad_request' | 'not_found' | 'unavailable'>>;
  /** Which of those this machine has; empty off macOS. */
  openInApps?(): Promise<{ ok: true; apps: ('vscode' | 'terminal' | 'claude')[] } | Fail<'bad_request'>>;
  /** Edit text in the person's editor and read it back. Absent in a shell that lacks it. */
  compose?(a: {
    text?: string;
    ext?: 'md' | 'txt';
  }): Promise<
    | { ok: true; text: string; changed: boolean }
    | Fail<
        | 'bad_request'
        | 'no_editor'
        | 'terminal_editor'
        | 'bad_editor'
        | 'not_waiting'
        | 'editor_failed'
        | 'cancelled'
        | 'busy'
        | 'too_large'
        | 'unavailable'
      >
  >;
  cancelCompose?(): Promise<{ ok: true } | Fail<'bad_request'>>;
  /** The system's chooser for one folder: its path, or null when the person cancels. Absent in a shell that lacks it (a plain browser). */
  pickFolder?(): Promise<{ ok: true; path: string | null } | Fail<'bad_request'>>;
  /** The page can draw <webview> elements: a web tab shows any site in one (an iframe otherwise). */
  webview?: boolean;
  /** A web tab's <webview> page as it is on screen, a PNG (base64), by its getWebContentsId(). */
  capturePreview?(id: number): Promise<{ ok: true; png: string } | Fail<'bad_request' | 'unavailable'>>;
  /** One element of that page, picked by the person's next click in it: the page as drawn then (a PNG, base64) and the element, cut
   * to size by the shell. `cancelled` when the person stops it, `timeout` when no click comes, `other_site` for a click inside a frame
   * from another site (not picked yet), `busy` while that page is already being picked from. */
  pickElement?(
    id: number,
  ): Promise<
    ({ ok: true; png: string } & PickedElement) | Fail<'bad_request' | 'busy' | 'cancelled' | 'timeout' | 'other_site' | 'unavailable'>
  >;
  /** Stops the pick on that page: its pickElement answers `cancelled`. The app window's Esc does not reach the page. */
  cancelPick?(id: number): Promise<{ ok: true } | Fail<'bad_request'>>;
  /** The booted iOS simulators and the Android devices adb sees, and whether idb is installed (a simulator is driven through it). */
  deviceList?(): Promise<
    { ok: true; devices: { kind: 'android' | 'ios'; id: string; name: string }[]; idb: boolean } | Fail<'bad_request' | 'unavailable'>
  >;
  /** The picture of one running Android emulator or device (adb) or iOS simulator (simctl) as a PNG (base64); `unavailable` says plainly why there is none. */
  deviceShot?(a: { kind: 'android' | 'ios'; id: string }): Promise<{ ok: true; png: string } | Fail<'bad_request' | 'unavailable'>>;
  /** A tap on that device at a place in its last picture, as fractions (0 to 1) of its width and height. */
  deviceTap?(a: { kind: 'android' | 'ios'; id: string; x: number; y: number }): Promise<{ ok: true } | Fail<'bad_request' | 'unavailable'>>;
  /** A drag on that device between two places of its last picture, as fractions, over `ms` milliseconds. */
  deviceSwipe?(a: {
    kind: 'android' | 'ios';
    id: string;
    x1: number;
    y1: number;
    x2: number;
    y2: number;
    ms?: number;
  }): Promise<{ ok: true } | Fail<'bad_request' | 'unavailable'>>;
  /** One line typed on that device. */
  deviceText?(a: { kind: 'android' | 'ios'; id: string; text: string }): Promise<{ ok: true } | Fail<'bad_request' | 'unavailable'>>;
  /** The follow-main build's commit and whether a newer main is waiting; `following` is false in any other build. */
  follow?(): Promise<
    | { ok: true; following: boolean; sha: string; state: 'current' | 'available' | 'updating' | 'ready' | 'failed'; next: string | null }
    | Fail<'bad_request'>
  >;
  /** The build the app is (its git sha, null when it has none) and whether the app can move the daemon onto it; the page compares the sha with the daemon's. */
  build?(): Promise<{ ok: true; sha: string | null; canRestart: boolean } | Fail<'bad_request'>>;
  /** Builds and restarts the daemon onto the app's build, as Update does; `cannot_restart` says why not. */
  restartDaemon?(): Promise<{ ok: true } | Fail<'bad_request' | 'cannot_restart'>>;
  /** The `hl` command an installed app carries: whether it has one, and whether it is linked into ~/.local/bin. */
  cli?(): Promise<{ ok: true; bundled: boolean; linked: boolean; link: string } | Fail<'bad_request'>>;
  /** Links the carried `hl` into ~/.local/bin; `cannot_link` says why not (another file is there). */
  linkCli?(): Promise<{ ok: true } | Fail<'bad_request' | 'cannot_link'>>;
  /** Looks at GitHub's main now; the answer is `follow()` again. */
  checkUpdate?(): Promise<{ ok: true } | Fail<'bad_request'>>;
  /** The update notice: where the update is, and its buttons. `onUpdate` is called now with the last state and on every change. */
  update?(): Promise<({ ok: true } & ShellUpdate) | Fail<'bad_request'>>;
  updateAct?(what: 'restart' | 'later' | 'retry' | 'details'): Promise<{ ok: true } | Fail<'bad_request'>>;
  onUpdate?(cb: (v: ShellUpdate) => void): () => void;
  /** The release channel the app reads updates from (null for a build that reads none), and picking another; it looks there at once. */
  updateChannel?(): Promise<{ ok: true; channel: UpdateChannel | null } | Fail<'bad_request'>>;
  setUpdateChannel?(channel: UpdateChannel): Promise<{ ok: true } | Fail<'bad_request'>>;
  /** The app's own daemon: its state now, its last state and every change, and a Start that brings it up again (a no-op while it starts or runs). */
  daemon?(): Promise<({ ok: true } & ShellDaemon) | Fail<'bad_request'>>;
  onDaemon?(cb: (v: ShellDaemon) => void): () => void;
  startDaemon?(): Promise<{ ok: true } | Fail<'bad_request'>>;
  /** Stops the app's own daemon, which stays stopped until started; `cannot_stop` says why not. */
  stopDaemon?(): Promise<{ ok: true } | Fail<'bad_request' | 'cannot_stop'>>;
  /** The owner's machines' doors from node.status (fp '' for a plain door), so the window may join whichever is master next. */
  keepDoors?(doors: { addr: string; fp: string }[]): Promise<{ ok: true } | Fail<'bad_request'>>;
  /** The clusters this Mac joined (docs/several-machines.md): each one's master, as the page opens it, and this Mac's device token
   * there. `onClusters` is called now with the list and on every change (a join, a leave, a master that answers or stops). */
  clusters?(): Promise<{ ok: true; clusters: ShellCluster[] } | Fail<'bad_request'>>;
  onClusters?(cb: (clusters: ShellCluster[]) => void): () => void;
  /** Joins the cluster whose Settings › Devices › Connect a device gave the link (or its address and the code), as `hl device join`
   * joins a machine: this person's Mac, on Off, or set to Run at once with `lending: 'run'`. `cannot_join` says why not; `cannot_run`
   * that it joined, on Off, and why Run was refused. */
  joinCluster?(a: {
    link?: string;
    address?: string;
    code?: string;
    lending?: 'run';
  }): Promise<({ ok: true } & ShellCluster) | Fail<'bad_request' | 'cannot_join' | 'cannot_run'>>;
  /** Leaves a joined cluster: Off there first (its agents here hand over), this Mac's device revoked, its engine stopped and removed.
   * Refused while this Mac is that cluster's master. */
  leaveCluster?(id: string): Promise<{ ok: true } | Fail<'bad_request' | 'cannot_leave'>>;
  /** Makes this Mac a joined cluster's master: its engine takes the board, handed over by the master or, when it is gone, from its copy
   * (node.promote); this Mac's own board is untouched. `cannot_promote` carries the engine's reason. */
  promoteCluster?(id: string): Promise<({ ok: true } & { number: number; handed_over: boolean }) | Fail<'bad_request' | 'cannot_promote'>>;
  /** The page's zoom factor (1 = 100%), read from the window itself: `outerWidth` lags a full-screen change, so the page cannot tell it from its own size. */
  zoom?(): number;
  /** The window entered or left full screen (the traffic lights are gone in it); called once with the current state on registering. Returns the way to stop listening. */
  onFullScreen?(cb: (full: boolean) => void): () => void;
  /** The shell has a new device token for this page (after a first pairing, or a daemon started again), or '' when it has none to give. Returns the way to stop listening. */
  onToken?(cb: (token: string) => void): () => void;
  /** settings.get through the shell's own daemon connection, for a window that is a paired device and may not read settings. Absent in a shell that lacks it. */
  settingsGet?(a?: {
    agent?: string;
  }): Promise<{ ok: true; result: Methods['settings.get']['result'] } | Fail<'bad_request' | 'refused' | 'unavailable'>>;
  /** settings.set the same way. */
  settingsSet?(a: {
    changes: PageSettings;
    agent?: string;
  }): Promise<{ ok: true; result: Methods['settings.set']['result'] } | Fail<'bad_request' | 'refused' | 'unavailable'>>;
  /** A notice was clicked. Returns the way to stop listening. */
  onNavigate(cb: (to: NavigateTo) => void): () => void;
}
