// The tray's menu as plain data, so a test can read it; own.ts hands it to Electron's Menu.
import { badgeText } from '#src/notices.ts';
import type { FollowState } from '#src/follow.ts';
import type { UpdatePhase } from '#src/update-flow.ts';
import type { Link } from '#src/mode.ts';
import type { DaemonState } from '#src/owner.ts';

export interface TrayItem {
  label?: string;
  type?: 'separator' | 'checkbox';
  checked?: boolean;
  enabled?: boolean;
  click?: () => void;
}

export interface TrayActions {
  open(): void;
  /** The Inbox, where the items waiting on the person are. */
  inbox(): void;
  startDaemon(): void;
  /** Absent when the daemon is another device's. */
  restartDaemon?(): void;
  /** Asks first (Stop daemon…); absent when the daemon is another device's. */
  stopDaemon?(): void;
  setOpenAtLogin(on: boolean): void;
  /** Asks first when the daemon runs (Quit Handline). */
  quit(): void;
  /** Puts an address or a commit on the clipboard. */
  copy(text: string): void;
}

/** What the menu-bar icon shows: nothing extra, a red dot (items wait on the person), a blue dot (an update is ready) or the yellow icon (trouble). */
export type TrayIcon = 'plain' | 'turn' | 'update' | 'trouble';

/** The update flow as the menu shows it. */
export interface TrayUpdate {
  phase: UpdatePhase;
  reason: string;
  restart(): void;
  retry(): void;
}

/** Follow-main mode: the commit the app runs, whether main has moved, and what the tray offers. */
export interface TrayFollow {
  sha: string;
  state: FollowState;
  /** The newer main's short sha, while there is one. */
  next: string | null;
  reason: string;
  check(): void;
  restart(): void;
}

/** What the tray shows besides the daemon's state. */
export interface TrayView {
  /** The My turn count. */
  unread: number;
  /** Agents mid-turn, null while not known. */
  working: number | null;
  openAtLogin: boolean;
  /** The other device this app is paired with, null for this Mac's daemon. */
  remote: string | null;
  /** The menu was opened with Option held: the addresses and the commit. */
  details: boolean;
  link?: Pick<Link, 'address' | 'version'>;
  update?: TrayUpdate;
  follow?: TrayFollow;
}

const agents = (n: number) => `${n} agent${n === 1 ? '' : 's'}`;

/** The first line, which says what runs. */
export function statusLine(state: DaemonState, reason: string, v: Pick<TrayView, 'working' | 'remote'>): string {
  if (v.remote) return `Paired with ${v.remote}`;
  if (state === 'running') return v.working === null ? 'Daemon running' : `Daemon running · ${agents(v.working)} working`;
  if (state === 'starting') return 'Starting daemon…';
  if (state === 'failed') return `Daemon stopped: ${reason || 'it did not start'}`;
  return reason ? `Daemon stopped: ${reason}` : 'Daemon stopped';
}

const down = (state: DaemonState, v: Pick<TrayView, 'remote'>) => !v.remote && (state === 'stopped' || state === 'failed');

/** What the icon carries: trouble first, then the person's turn, then an update. */
export function trayIcon(state: DaemonState, v: TrayView): TrayIcon {
  if (down(state, v)) return 'trouble';
  if (v.unread > 0) return 'turn';
  return v.update?.phase === 'ready' || v.update?.phase === 'later' ? 'update' : 'plain';
}

/** What Option shows and the usual menu leaves out: the daemon's address and build, and the commit the app follows. They copy on a click. */
function detailItems(a: TrayActions, v: TrayView): TrayItem[] {
  const items: TrayItem[] = [];
  const l = v.link;
  if (l?.address) items.push({ label: `${l.address}${l.version ? ` · ${l.version}` : ''}`, click: () => a.copy(l.address!) });
  const f = v.follow;
  if (f) items.push({ label: `Handline is on main ${f.sha.slice(0, 7)}`, click: () => a.copy(f.sha) });
  if (f && f.state !== 'ready') items.push({ label: 'Check for update', click: f.check });
  return items;
}

/** The menu, top to bottom: what runs, Open, what needs the person, Open at login, the daemon's controls, Quit. */
export function trayMenu(state: DaemonState, reason: string, a: TrayActions, v: TrayView): TrayItem[] {
  const items: TrayItem[] = [{ label: statusLine(state, reason, v), enabled: false }, { type: 'separator' }];
  items.push({ label: 'Open Handline', click: a.open });
  if (v.unread > 0) items.push({ label: `Needs you · ${badgeText(v.unread)}`, click: a.inbox });
  if (down(state, v)) items.push({ label: 'Start daemon', click: a.startDaemon });
  const u = v.update;
  if (u?.phase === 'preparing') items.push({ label: 'Updating Handline…', enabled: false });
  if (u?.phase === 'ready' || u?.phase === 'later')
    items.push({ label: 'Handline is ready to restart', enabled: false }, { label: 'Restart now', click: u.restart });
  if (u?.phase === 'failed')
    items.push({ label: `Handline was not updated: ${u.reason}`, enabled: false }, { label: 'Try again', click: u.retry });
  items.push({ type: 'separator' });
  items.push({ label: 'Open at login', type: 'checkbox', checked: v.openAtLogin, click: () => a.setOpenAtLogin(!v.openAtLogin) });
  if (!v.remote && state === 'running' && (a.restartDaemon || a.stopDaemon)) {
    items.push({ type: 'separator' });
    if (a.restartDaemon) items.push({ label: 'Restart daemon', click: a.restartDaemon });
    if (a.stopDaemon) items.push({ label: 'Stop daemon…', click: a.stopDaemon });
  }
  if (v.details) {
    const more = detailItems(a, v);
    if (more.length) items.push({ type: 'separator' }, ...more);
  }
  items.push({ type: 'separator' }, { label: 'Quit Handline', click: a.quit });
  return items;
}
