// The owner's machines as the shell sees them (docs/story4-design.md section 4): the doors the page may join whichever is master at,
// and whether this Mac's own daemon is a runner that "Make this Mac the master" can promote.
import fs from 'node:fs';
import path from 'node:path';
import { DaemonClient, masterOf, type Transport } from '@handline/client';
import type { ShellDaemon } from '@handline/app-core';
import type { Door, Remote, Settings, SettingsFile } from '#src/mode.ts';
import { CHANNELS, DaemonView } from '#src/bridge.ts';
import type { Own } from '#src/own.ts';
import { ipcMain, type App, type WebContents } from 'electron';

/** A paired remote daemon and the owner's other machines have self-signed certificates: the page trusts each by the fingerprint
 * pairing or node.status handed over, and no other; only the app window joins the owner's machines. */
export function trustFingerprints(
  app: Pick<App, 'on'>,
  doors: Doors,
  o: { remote: Remote | null; window: () => WebContents | undefined },
): Doors {
  app.on('certificate-error', (e, wc, url, _err, cert, trust) => {
    e.preventDefault();
    const got = Buffer.from(cert.fingerprint.replace(/^sha256\//, ''), 'base64').toString('hex');
    const host = new URL(url).host;
    const r = o.remote;
    trust((!!r?.fingerprint && host === r.host && got === r.fingerprint) || (wc === o.window() && doors.trusts(host, got)));
  });
  return doors;
}

/** The doors the page found (keepDoors), host:port to certificate fingerprint ('' for a plain door), kept for the next launch. */
export class Doors {
  private map = new Map<string, string>();
  private file: Pick<SettingsFile, 'read' | 'write'>;
  constructor(file: Pick<SettingsFile, 'read' | 'write'>, kept: Settings) {
    this.file = file;
    for (const d of kept.doors ?? []) this.map.set(d.addr, d.fp);
  }
  has(host: string): boolean {
    return this.map.has(host);
  }
  /** The certificate at host is the one its node.status named. */
  trusts(host: string, fingerprint: string): boolean {
    const fp = this.map.get(host);
    return !!fp && fp === fingerprint;
  }
  keep(list: Door[]): void {
    const next = new Map(list.map((d) => [d.addr, d.fp]));
    if (next.size === this.map.size && [...next].every(([a, fp]) => this.map.get(a) === fp)) return;
    this.map = next;
    this.file.write({ ...this.file.read(), doors: [...next].map(([addr, fp]) => ({ addr, fp })) });
  }
}

/** A call on its own short connection to this Mac's daemon: a runner refuses the shell connection's subscriptions, so that one never
 * opens there. */
export async function onOwnDaemon<T>(
  o: { connect: () => Promise<Transport>; key: () => string | null },
  f: (c: DaemonClient) => Promise<T>,
): Promise<T> {
  const c = new DaemonClient({ client: 'electron', connect: o.connect, key: o.key, reconnect: false });
  try {
    await c.start();
    return await f(c);
  } finally {
    c.close();
  }
}

/** Whether this Mac's daemon is a runner (a hello that names no master but serves node.promote), looked at again on each change of
 * the daemon, and its promote. `changed` is told when the answer changes. */
export function ownRunner(o: { connect: () => Promise<Transport>; key: () => string | null; home: string; changed: () => void }) {
  let runner: { node: string } | null = null;
  let looks = 0; // only the newest look counts
  const ask = () =>
    onOwnDaemon(o, async (c) => {
      if (masterOf(c.hello) || !c.hello?.methods.some((m) => m.name === 'node.promote')) return null; // a master, or a daemon from before
      const node = fs.readFileSync(path.join(o.home, 'node_id'), 'utf8').trim();
      return node ? { node } : null;
    }).catch(() => null);
  const set = (r: { node: string } | null) => {
    if (r?.node === runner?.node) return;
    runner = r;
    o.changed();
  };
  return {
    runner: () => runner,
    look: () => {
      const n = ++looks;
      void ask().then((r) => n === looks && set(r));
    },
    promote: async () => {
      const r = await onOwnDaemon(o, (c) => c.call('node.promote', {}));
      looks++; // a look begun before is stale: it starts again as master
      set(null);
      return r;
    },
  };
}

/** The page's calls on this Mac's daemon: its state (with whether it is a runner), Start, Stop, Make this Mac the master, and the doors
 * the page hands over. */
export function serveOwnDaemon(o: {
  origin: string;
  own: Pick<Own, 'daemon' | 'startDaemon' | 'stopDaemon' | 'onDaemon'>;
  /** The shell's own connection: its state changes as the daemon starts again as what it is now (made the master, or outranked). */
  shellClient: Pick<DaemonClient, 'onState'>;
  connect: () => Promise<Transport>;
  key: () => string | null;
  home: string;
  doors: () => Doors | null;
  send: (v: ShellDaemon) => void;
  /** After this Mac was made the master (a runner Mac's shell then starts again on its own door). */
  promoted?: () => void;
}): void {
  const view = (): ShellDaemon => ({ ...o.own.daemon(), runner: mine.runner() });
  const mine = ownRunner({ ...o, changed: () => o.send(view()) });
  mine.look();
  const d = new DaemonView(o.origin, {
    view,
    start: () => o.own.startDaemon(),
    stop: () => o.own.stopDaemon(),
    promote: async () => {
      const r = await mine.promote();
      o.promoted?.();
      return r;
    },
    keepDoors: (list) => o.doors()?.keep(list),
  });
  ipcMain.handle(CHANNELS.daemon, (e) => d.view(e));
  ipcMain.handle(CHANNELS.startDaemon, (e) => d.start(e));
  ipcMain.handle(CHANNELS.stopDaemon, (e) => d.stop(e));
  ipcMain.handle(CHANNELS.promote, (e) => d.promote(e));
  ipcMain.handle(CHANNELS.keepDoors, (e, arg) => d.keepDoors(e, arg));
  o.own.onDaemon(() => (o.send(view()), mine.look()));
  o.shellClient.onState(mine.look);
}
