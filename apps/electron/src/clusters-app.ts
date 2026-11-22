// The joined clusters in Electron terms (clusters.ts holds the logic a test drives): each engine kept as owner.ts keeps a daemon,
// `hl device join` run on its home, the shell's connection to each master, and the page's calls.
import { execFile } from 'node:child_process';
import os from 'node:os';
import { ipcMain } from 'electron';
import { DaemonClient } from '@handline/client';
import { ClusterCalls, CHANNELS } from '#src/bridge.ts';
import { Clusters } from '#src/clusters.ts';
import type { SettingsFile } from '#src/mode.ts';
import { engineDaemon, engineEnv, type LoginPath } from '#src/own.ts';
import { protectedDir } from '#src/pair.ts';
import { joinedOf, masterTransport } from '#src/joined.ts';

/** `hl device join <link>` on an engine's home, the one way a machine joins a master; rejects with what hl said. */
const hlJoin = (hl: string, link: string, home: string): Promise<void> =>
  new Promise((resolve, reject) =>
    execFile(hl, ['device', 'join', link], { env: engineEnv(home), timeout: 30_000, encoding: 'utf8' }, (err, _out, errOut) =>
      err
        ? reject(
            new Error(
              (errOut || err.message)
                .trim()
                .split('\n')
                .pop()!
                .replace(/^join refused: /, ''),
            ),
          )
        : resolve(),
    ),
  );

export function serveClusters(o: {
  origin: string;
  /** This Mac's own home. */
  home: string;
  settings: SettingsFile;
  /** The hl this app carries (a checkout's go/hl). */
  hl: string;
  env: (k: string) => string | undefined;
  root: string;
  loginPath: () => Promise<LoginPath>;
  send: (v: unknown) => void;
}): Clusters {
  const clusters: Clusters = new Clusters({
    home: o.home,
    file: o.settings,
    join: (link, home) => hlJoin(o.hl, link, home),
    joined: (home) => joinedOf(os.homedir(), home),
    store: (home) => protectedDir(os.homedir(), home),
    engine: (home) => engineDaemon({ env: o.env, root: o.root, home, loginPath: o.loginPath }),
    link: (j) => new DaemonClient({ client: 'electron', connect: masterTransport(j), token: j.master.token }),
    changed: () => o.send(clusters.list()),
  });
  const calls = new ClusterCalls(o.origin, {
    list: () => clusters.list(),
    join: (i) => clusters.join(i),
    leave: (id) => clusters.leave(id),
  });
  ipcMain.handle(CHANNELS.clusters, (e) => calls.list(e));
  ipcMain.handle(CHANNELS.joinCluster, (e, arg) => calls.join(e, arg));
  ipcMain.handle(CHANNELS.leaveCluster, (e, arg) => calls.leave(e, arg));
  return clusters;
}
