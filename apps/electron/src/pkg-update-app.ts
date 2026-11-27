// The installed app's update in Electron terms: the network, the files and the restart behind PkgPorts (pkg-update.ts).
import fs from 'node:fs';
import path from 'node:path';
import { app, ipcMain } from 'electron';
import { CHANNELS, UpdateChannelCalls } from '#src/bridge.ts';
import { downloadTo, sha512Of, unpackApp } from '#src/pkg-files.ts';
import { SelfUpdater, type PkgPorts } from '#src/pkg-update.ts';
import { leftovers, STAGING, swapBundle } from '#src/swap.ts';
import type { UpdateFlow } from '#src/update-flow.ts';
import { quitQuietly } from '#src/ask.ts';
import { channelOf, withChannel, type UpdateChannel } from '#src/update-channel.ts';

/** The installed Handline.app: three folders above the executable (Contents/MacOS/Handline). */
const installedApp = () => path.resolve(app.getPath('exe'), '../../..');

/** The address the build reads its updates from, written into its package.json by electron-builder (electron-builder.config.mjs); null for any other build. */
export function packagedUpdateUrl(appPath: string): string | null {
  try {
    const url = (JSON.parse(fs.readFileSync(path.join(appPath, 'package.json'), 'utf8')) as { handlineUpdateUrl?: unknown })
      .handlineUpdateUrl;
    return typeof url === 'string' && url ? url : null;
  } catch {
    return null;
  }
}

/** Removes what an earlier swap left beside the app. */
export function sweepSwap(): void {
  const parent = path.dirname(installedApp());
  try {
    for (const p of leftovers(parent, fs.readdirSync(parent), path.basename(installedApp())))
      fs.rmSync(p, { recursive: true, force: true });
  } catch {
    /* the folder cannot be listed: nothing to remove */
  }
}

function ports(o: { idle: () => Promise<boolean> }): PkgPorts {
  return {
    current: () => app.getVersion(),
    arch: () => (process.arch === 'x64' ? 'x64' : 'arm64'),
    text: async (url) => {
      const r = await fetch(url);
      if (!r.ok) throw new Error(`${url}: ${r.status}`);
      return r.text();
    },
    download: (url, progress) => downloadTo(app.getPath('temp'), url, progress),
    sha512: sha512Of,
    stage: (zip) => unpackApp(zip, path.join(path.dirname(installedApp()), STAGING), path.basename(installedApp())),
    idle: o.idle,
    // The daemon keeps running through the swap; the relaunched app restarts it on the new bundle's handlined, a build of its own.
    swapAndRestart: async (staged) => {
      swapBundle(installedApp(), staged);
      app.relaunch();
      quitQuietly(app);
    },
  };
}

/** Looks for a newer build once a minute after launch and then every half hour, on the release of the channel the person picked (`picked`), else the
 * build's own. `channel` is the one in use and `source` its address, null for a build with no update address (a checkout); `look` checks at once. */
export function startPkgUpdate(o: { flow: UpdateFlow; idle: () => Promise<boolean>; picked: () => UpdateChannel | undefined }): {
  stop: () => void;
  look: () => void;
  channel: () => UpdateChannel | null;
  source: () => string | null;
} {
  const base = packagedUpdateUrl(app.getAppPath());
  const built = base ? channelOf(base) : null;
  if (!base || !built) return { stop: () => undefined, look: () => undefined, channel: () => null, source: () => null };
  sweepSwap();
  const channel = () => o.picked() ?? built;
  const updater = new SelfUpdater(ports(o), o.flow, () => ({ base: withChannel(base, channel()), switched: channel() !== built }));
  const look = () => void updater.check();
  const first = setTimeout(look, 60_000);
  const every = setInterval(look, 30 * 60_000);
  return {
    stop: () => {
      clearTimeout(first);
      clearInterval(every);
    },
    look,
    channel,
    source: () => withChannel(base, channel()),
  };
}

/** Settings' Updates channel: which release the app reads, and picking another. */
export function handleUpdateChannel(
  origin: string,
  pkg: { channel: () => UpdateChannel | null; source: () => string | null; look: () => void },
  set: (c: UpdateChannel) => void,
): void {
  const look = pkg.look;
  const u = new UpdateChannelCalls(origin, {
    get: pkg.channel,
    version: () => (app.isPackaged ? app.getVersion() : null),
    source: pkg.source,
    set: (c) => {
      set(c);
      look();
    },
  });
  ipcMain.handle(CHANNELS.updateChannel, (e) => u.get(e));
  ipcMain.handle(CHANNELS.setUpdateChannel, (e, arg) => u.set(e, arg));
}
