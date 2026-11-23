// The joined clusters in Electron terms (clusters.ts holds the logic a test drives): each engine kept as owner.ts keeps a daemon,
// `hl device join` run on its home, the shell's connection to each master, and the page's calls.
import { execFile } from 'node:child_process';
import os from 'node:os';
import path from 'node:path';
import { ipcMain } from 'electron';
import { DaemonClient } from '@handline/client';
import { unixSocketTransport } from '@handline/client-node';
import { ClusterCalls, CHANNELS } from '#src/bridge.ts';
import type { ShellCluster } from '@handline/app-core';
import { Clusters, engineEnv } from '#src/clusters.ts';
import type { SettingsFile } from '#src/mode.ts';
import { engineDaemon, readFounderKey, type LoginPath } from '#src/own.ts';
import { freePort } from '#src/dev-team.ts';
import { protectedDir } from '#src/pair.ts';
import { joinedOf, ledOf, masterTransport } from '#src/joined.ts';

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

/** node.promote on an engine's socket, as its founder: the engine starts again as master once it has answered. */
async function promoteEngine(home: string): Promise<{ number: number; handed_over: boolean }> {
  const c = new DaemonClient({
    client: 'electron',
    connect: () => unixSocketTransport(path.join(home, 'daemon.sock')),
    key: () => readFounderKey(home),
    reconnect: false,
  });
  try {
    await c.start();
    return await c.call('node.promote', {});
  } finally {
    c.close();
  }
}

export function serveClusters(o: {
  origin: string;
  /** This Mac's own home. */
  home: string;
  /** The host this Mac's daemon listens on: each engine listens there on a port of its own. */
  listenHost: string;
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
    joined: (home, port) => joinedOf(os.homedir(), home) ?? (port ? ledOf(os.homedir(), home, `127.0.0.1:${port}`) : null),
    store: (home) => protectedDir(os.homedir(), home),
    engine: (home, port) =>
      engineDaemon({
        env: o.env,
        root: o.root,
        home,
        listen: port ? `${o.listenHost}:${port}` : undefined,
        ownHome: o.home,
        loginPath: o.loginPath,
      }),
    // The token stays the same as the master moves: the engine made the master keeps the one it joined with as its own device's.
    link: (current) => new DaemonClient({ client: 'electron', connect: () => masterTransport(current())(), token: current().master.token }),
    port: freePort,
    promote: promoteEngine,
    changed: () => o.send(clusters.list()),
  });
  // tools/app.mjs --fake: the clusters its second fake daemon stands for, listed as joined; Leave drops one, a join is refused.
  const fake = o.env('FAKE_CLUSTERS');
  let fakes: ShellCluster[] = fake ? (JSON.parse(fake) as ShellCluster[]) : [];
  const calls = new ClusterCalls(
    o.origin,
    fake
      ? {
          list: () => fakes,
          join: () => Promise.reject(new Error('the test app joins no cluster')),
          leave: async (id) => {
            fakes = fakes.filter((c) => c.id !== id);
            o.send(fakes);
          },
          promote: () => Promise.reject(new Error('the test app moves no master')),
        }
      : {
          list: () => clusters.list(),
          join: (i) => clusters.join(i),
          leave: (id) => clusters.leave(id),
          promote: (id) => clusters.promote(id),
        },
  );
  ipcMain.handle(CHANNELS.clusters, (e) => calls.list(e));
  ipcMain.handle(CHANNELS.joinCluster, (e, arg) => calls.join(e, arg));
  ipcMain.handle(CHANNELS.leaveCluster, (e, arg) => calls.leave(e, arg));
  ipcMain.handle(CHANNELS.promoteCluster, (e, arg) => calls.promote(e, arg));
  return clusters;
}
