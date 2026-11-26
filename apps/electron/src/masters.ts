// The owner's machines as the shell sees them (docs/several-machines.md section 4): the doors the page may join whichever is master at.
import type { ShellDaemon } from '@handline/app-core';
import type { Door, Settings, SettingsFile } from '#src/mode.ts';
import { CHANNELS, DaemonView } from '#src/bridge.ts';
import type { Own } from '#src/own.ts';
import { ipcMain, type App, type WebContents } from 'electron';

/** The owner's machines and the joined clusters' masters have self-signed certificates: the app window trusts each by the fingerprint
 * node.status or the join handed over, and no other. */
export function trustFingerprints(app: Pick<App, 'on'>, doors: Doors, window: () => WebContents | undefined): Doors {
  app.on('certificate-error', (e, wc, url, _err, cert, trust) => {
    e.preventDefault();
    const got = Buffer.from(cert.fingerprint.replace(/^sha256\//, ''), 'base64').toString('hex');
    trust(wc === window() && doors.trusts(new URL(url).host, got));
  });
  return doors;
}

/** The doors the page may open, host:port to certificate fingerprint ('' for a plain door): those it found (keepDoors), kept for the
 * next launch, and the joined clusters' (`also`). */
export class Doors {
  private map = new Map<string, string>();
  private file: Pick<SettingsFile, 'read' | 'write'>;
  private also: () => Door[];
  constructor(file: Pick<SettingsFile, 'read' | 'write'>, kept: Settings, also: () => Door[] = () => []) {
    this.file = file;
    this.also = also;
    for (const d of kept.doors ?? []) this.map.set(d.addr, d.fp);
  }
  private fp(host: string): string | undefined {
    return this.map.get(host) ?? this.also().find((d) => d.addr === host)?.fp;
  }
  has(host: string): boolean {
    return this.fp(host) !== undefined;
  }
  /** The certificate at host is the one its node.status or its join named. */
  trusts(host: string, fingerprint: string): boolean {
    const fp = this.fp(host);
    return !!fp && fp === fingerprint;
  }
  keep(list: Door[]): void {
    const next = new Map(list.map((d) => [d.addr, d.fp]));
    if (next.size === this.map.size && [...next].every(([a, fp]) => this.map.get(a) === fp)) return;
    this.map = next;
    this.file.write({ ...this.file.read(), doors: [...next].map(([addr, fp]) => ({ addr, fp })) });
  }
}

/** The page's calls on this Mac's daemon: its state, Start, Stop, and the doors the page hands over. */
export function serveOwnDaemon(o: {
  origin: string;
  own: Pick<Own, 'daemon' | 'startDaemon' | 'stopDaemon' | 'onDaemon'>;
  doors: () => Doors | null;
  send: (v: ShellDaemon) => void;
}): void {
  const d = new DaemonView(o.origin, {
    view: () => o.own.daemon(),
    start: () => o.own.startDaemon(),
    stop: () => o.own.stopDaemon(),
    keepDoors: (list) => o.doors()?.keep(list),
  });
  ipcMain.handle(CHANNELS.daemon, (e) => d.view(e));
  ipcMain.handle(CHANNELS.startDaemon, (e) => d.start(e));
  ipcMain.handle(CHANNELS.stopDaemon, (e) => d.stop(e));
  ipcMain.handle(CHANNELS.keepDoors, (e, arg) => d.keepDoors(e, arg));
  o.own.onDaemon(() => o.send(o.own.daemon()));
}
