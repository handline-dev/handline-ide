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
  setOpenAtLogin(on: boolean): void;
  /** Asks first when the daemon runs (Quit Handline). */
  quit(): void;
  /** Puts an address or a commit on the clipboard. */
  copy(text: string): void;
}

/** What the menu-bar icon shows: nothing extra, a red dot (items wait on the person), a blue dot (an update is ready) or the yellow icon (trouble). */
export type TrayIcon = 'plain' | 'turn' | 'update' | 'trouble';

/** The update flow as the tray reads it: a ready update is a blue dot and a Restart to update item, a failed one trouble. */
export interface TrayUpdate {
  phase: UpdatePhase;
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
  /** The menu was opened with Option held: the addresses and the commit. */
  details: boolean;
  link?: Pick<Link, 'address' | 'version'>;
  update?: TrayUpdate;
  follow?: TrayFollow;
}

const ready = (u: TrayUpdate) => u.phase === 'ready' || u.phase === 'later';

const agents = (n: number) => `${n} agent${n === 1 ? '' : 's'}`;

const daemonDown = (state: DaemonState) => state !== 'running' && state !== 'starting';

/** Something the person should know about: this Mac's daemon is not running, or an update failed. */
const trouble = (state: DaemonState, v: Pick<TrayView, 'update'>) => daemonDown(state) || v.update?.phase === 'failed';

/** The first line: what runs, or what is wrong, named. */
export function statusLine(state: DaemonState, v: Pick<TrayView, 'working' | 'update'>): string {
  if (state === 'failed') return 'Handline failed to start';
  if (state === 'stopped') return 'Handline is stopped';
  if (v.update?.phase === 'failed') return 'Update failed · Try again';
  if (state === 'starting') return 'Handline is starting…';
  return v.working ? `Handline · ${agents(v.working)} working` : 'Handline';
}

/** What the icon carries: trouble first, then the person's turn, then an update. */
export function trayIcon(state: DaemonState, v: TrayView): TrayIcon {
  if (trouble(state, v)) return 'trouble';
  if (v.unread > 0) return 'turn';
  return v.update && ready(v.update) ? 'update' : 'plain';
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

/** What the trouble line's click does: a failed update tries again; a daemon that is down opens the app, which says why. */
const fix = (state: DaemonState, a: TrayActions, v: TrayView) => (!daemonDown(state) && v.update ? v.update.retry : a.open);

/** The menu, top to bottom: what runs (or that the app needs attention, which opens it), Open, what needs the person, Open at login, Quit. */
export function trayMenu(state: DaemonState, a: TrayActions, v: TrayView): TrayItem[] {
  const items: TrayItem[] = [
    trouble(state, v) ? { label: statusLine(state, v), click: fix(state, a, v) } : { label: statusLine(state, v), enabled: false },
    { type: 'separator' },
  ];
  items.push({ label: 'Open Handline', click: a.open });
  if (v.update && ready(v.update)) items.push({ label: 'Restart to update', click: v.update.restart });
  if (v.unread > 0) items.push({ label: `Needs you · ${badgeText(v.unread)}`, click: a.inbox });
  items.push({ type: 'separator' });
  items.push({ label: 'Open at login', type: 'checkbox', checked: v.openAtLogin, click: () => a.setOpenAtLogin(!v.openAtLogin) });
  if (v.details) {
    const more = detailItems(a, v);
    if (more.length) items.push({ type: 'separator' }, ...more);
  }
  items.push({ type: 'separator' }, { label: 'Quit Handline', click: a.quit });
  return items;
}
