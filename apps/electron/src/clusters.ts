// The clusters this Mac joined (docs/story4-design.md section 1, slice 24). This Mac's own daemon keeps its board; for each cluster the
// shell runs an engine beside it: a handlined in a home of its own that joined that cluster's master as `hl device join` joins a
// machine (this person's Mac, on Off), so it runs that cluster's agents only once it is set to Run there. The page reaches each master
// itself, its certificate pinned, with the engine's device token; the shell keeps a connection to each to say whether it answers.
// "Make this Mac the master" of a cluster makes its engine that cluster's master in its own home (node.promote on its socket); this Mac's
// own daemon and board stay as they are.
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import type { ShellCluster } from '@handline/app-core';
import type { DaemonClient } from '@handline/client';
import { joinLink, masterUrl, type Door, type KeptCluster, type OfferInput, type SettingsFile } from '#src/mode.ts';
import type { Joined } from '#src/joined.ts';
import type { Cmd } from '#src/owner.ts';

/** The engine's daemon, as owner.ts keeps one. */
export interface Engine {
  start(): Promise<void>;
  stop(): Promise<void>;
  /** Stops looking after it, once it is stopped for good. */
  close(): void;
  readonly state: string;
  readonly reason: string;
}

/** The shell's connection to a cluster's master with the engine's device token. */
export type MasterLink = Pick<DaemonClient, 'start' | 'close' | 'call' | 'onState' | 'state' | 'hello'>;

export interface ClusterDeps {
  /** This Mac's own home: the engines' homes are beside it. */
  home: string;
  file: Pick<SettingsFile, 'read' | 'write'>;
  /** `hl device join <link>` on an engine's home; rejects with hl's reason. */
  join(link: string, home: string): Promise<void>;
  /** What the engine's home joined (joined.ts joinedOf), or, made that cluster's master, itself at its port; null for neither. */
  joined(home: string, port: number | undefined): Joined | null;
  /** The engine's protected store, removed with its home on Leave. */
  store(home: string): string;
  /** Listens on `port` to serve as master once made it; none for a cluster kept without one. */
  engine(home: string, port: number | undefined): Engine;
  /** Each connect goes by what the home holds then: a master that moved, or this Mac's engine made it. */
  link(current: () => Joined): MasterLink;
  /** A free port for a new engine. */
  port(): Promise<number>;
  /** node.promote on the engine's socket. */
  promote(home: string): Promise<{ number: number; handed_over: boolean }>;
  /** The list changed: a join, a leave, a master that answers or stops answering, a name learned. */
  changed(): void;
}

/** Where the engine of cluster `id` keeps its home: beside this Mac's own (~/.handline-clusters/<id>). */
export const engineHome = (home: string, id: string): string => path.join(`${path.resolve(home)}-clusters`, id);

interface Live {
  id: string;
  home: string;
  port: number | undefined;
  /** Read again at each change of the connection: its master and the owner's machines change as it moves. */
  joined: Joined;
  engine: Engine;
  starting: Promise<void>;
  link: MasterLink;
  /** A promote not answered yet: Leave and a second promote wait for its answer. */
  promoting: boolean;
}

export class Clusters {
  private live = new Map<string, Live>();
  private d: ClusterDeps;
  constructor(d: ClusterDeps) {
    this.d = d;
  }

  /** At launch: an engine and a connection for each cluster kept. A kept one whose home joined nothing is dropped; one kept without a
   * port (joined before engines listened) gets one. */
  async start(): Promise<void> {
    for (const c of this.kept()) {
      const home = engineHome(this.d.home, c.id);
      const port = c.port ?? (await this.d.port());
      const j = this.d.joined(home, port);
      if (!j) {
        this.keep(this.kept().filter((k) => k.id !== c.id));
        continue;
      }
      if (port !== c.port) this.keep(this.kept().map((k) => (k.id === c.id ? { ...k, port } : k)));
      this.run(c.id, home, port, j);
    }
  }

  list(): ShellCluster[] {
    const names = new Map(this.kept().map((c) => [c.id, c.name]));
    return [...this.live.values()].map((l) => ({
      id: l.id,
      name: names.get(l.id) || l.joined.master.host,
      daemon: masterUrl(l.joined.master),
      token: l.joined.master.token,
      reachable: l.link.state === 'open',
      master: !!l.joined.led,
    }));
  }

  /** The doors the page may open, each with the certificate it pins: every cluster's master and the owner's machines it named. */
  doors(): Door[] {
    return [...this.live.values()].flatMap((l) => [
      { addr: l.joined.master.host, fp: l.joined.master.fingerprint ?? '' },
      ...l.joined.doors,
    ]);
  }

  /** Joins the cluster whose Connect a device made the link: a new engine home joins its master, then its engine starts. */
  async join(i: OfferInput): Promise<ShellCluster> {
    const link = joinLink(i); // a bad link throws before anything is made
    const id = crypto.randomBytes(4).toString('hex');
    const port = await this.d.port();
    const home = engineHome(this.d.home, id);
    fs.mkdirSync(path.dirname(home), { recursive: true });
    fs.mkdirSync(home); // throws for an id taken already, whose home is not this join's to remove
    let j: Joined | null;
    try {
      await this.d.join(link, home);
      j = this.d.joined(home, port);
      if (!j) throw new Error('the join kept no credential');
    } catch (e) {
      this.remove(home);
      throw e;
    }
    this.keep([...this.kept(), { id, name: '', port }]);
    return this.run(id, home, port, j);
  }

  /** Leaves a cluster: Off there, so its agents here hand over, then this Mac's device revoked, the engine stopped and its home
   * removed. A master that does not answer, or refuses, is no reason to stay: the device stays listed there until its owner revokes it. */
  async leave(id: string): Promise<void> {
    const l = this.live.get(id);
    if (!l) throw new Error('this Mac has not joined that cluster');
    if (l.joined.led) throw new Error("this Mac is that cluster's master: make another of its machines the master first");
    if (l.promoting) throw new Error("this Mac is being made that cluster's master");
    this.live.delete(id); // a second Leave meanwhile finds nothing to leave
    this.d.changed();
    if (l.link.state === 'open') {
      // The revoke ends this very connection, and a call on a lost one waits for a reconnect that never comes: closing it fails the call.
      l.link.onState((s) => s !== 'open' && l.link.close());
      const device = l.joined.master.device;
      await l.link.call('device.set', { device, lending: 'off', request_id: `leave-off-${id}` }).catch(() => undefined);
      await l.link.call('device.revoke', { device, request_id: `leave-revoke-${id}` }).catch(() => undefined);
    }
    l.link.close();
    await l.starting; // a stop asked while it starts would wait on the start and stop nothing
    await l.engine.stop();
    if (l.engine.state !== 'stopped') {
      this.live.set(id, l); // still listed, so Leave can be tried again
      this.d.changed();
      throw new Error(`its engine did not stop: ${l.engine.reason}`);
    }
    l.engine.close();
    this.keep(this.kept().filter((k) => k.id !== id));
    this.remove(l.home);
  }

  /** Makes this Mac that cluster's master: its engine takes the board over, handed by the master or, when it is gone, from its copy, and
   * starts again as master. The connection follows by itself, as it follows any master that moves. */
  async promote(id: string): Promise<{ number: number; handed_over: boolean }> {
    const l = this.live.get(id);
    if (!l) throw new Error('this Mac has not joined that cluster');
    if (l.joined.led) throw new Error("this Mac is that cluster's master already");
    if (l.promoting) throw new Error("this Mac is being made that cluster's master");
    l.promoting = true;
    try {
      return await this.d.promote(l.home);
    } finally {
      l.promoting = false;
      l.joined = this.d.joined(l.home, l.port) ?? l.joined;
      this.d.changed();
    }
  }

  private run(id: string, home: string, port: number | undefined, joined: Joined): ShellCluster {
    const engine = this.d.engine(home, port);
    const now = { joined }; // the link reads it at each connect, so it exists before the link
    const link = this.d.link(() => now.joined);
    const l: Live = Object.assign(now, { id, home, port, engine, starting: engine.start(), link, promoting: false });
    this.live.set(id, l);
    link.onState((s) => {
      l.joined = this.d.joined(home, port) ?? l.joined;
      if (s === 'open') this.learnName(id, link);
      this.d.changed();
    });
    void untilUp(() => (this.live.get(id) === l ? link.start() : Promise.resolve()));
    this.d.changed();
    return this.list().find((c) => c.id === id)!;
  }

  /** The cluster's name, as its master's hello gives it, kept for when it does not answer. */
  private learnName(id: string, link: MasterLink): void {
    const name = (link.hello?.capabilities as { cluster_name?: unknown } | undefined)?.cluster_name;
    if (typeof name !== 'string' || !name) return;
    const kept = this.kept();
    if (kept.find((c) => c.id === id)?.name === name) return;
    this.keep(kept.map((c) => (c.id === id ? { ...c, name } : c)));
  }

  private kept(): KeptCluster[] {
    return this.d.file.read().clusters ?? [];
  }

  private keep(clusters: KeptCluster[]): void {
    this.d.file.write({ ...this.d.file.read(), clusters });
  }

  private remove(home: string): void {
    fs.rmSync(this.d.store(home), { recursive: true, force: true });
    fs.rmSync(home, { recursive: true, force: true });
  }
}

/** Runs `f` until it stops throwing, two seconds apart. */
export async function untilUp(f: () => Promise<unknown>): Promise<void> {
  for (;;) {
    try {
      return void (await f());
    } catch {
      await new Promise((r) => setTimeout(r, 2000));
    }
  }
}

/** The environment of a cluster engine's processes (its daemon, and the hl that joins it): this app's, on the engine's home, with
 * none of the HANDLINE_ variables that name this Mac's own daemon (its socket, listener, tmux server, binaries); a test's stand-in
 * user home stays. */
export function engineEnv(home: string, pathVar?: string): NodeJS.ProcessEnv {
  const env = Object.fromEntries(Object.entries(process.env).filter(([k]) => !k.startsWith('HANDLINE_') || k === 'HANDLINE_TEST_ROOT'));
  return { ...env, ...(pathVar ? { PATH: pathVar } : {}), HANDLINE_HOME: home };
}

export interface EngineOpts {
  env: (k: string) => string | undefined;
  root: string;
  home: string;
  listen: string | undefined;
  /** This Mac's own home: the engine serves the network_addresses chosen there, read live, never copied into its own. */
  ownHome: string;
}

/** The engine's command: its home, its door once made the master, and the addresses this Mac's own daemon was told to serve. */
export function engineCmd(o: EngineOpts, pathVar: string): Cmd {
  return {
    file: o.env('HANDLINED') ?? `${o.root}/go/handlined`,
    args: ['-home', o.home, ...(o.listen ? ['-listen', o.listen, '-addresses-home', o.ownHome] : [])],
    env: engineEnv(o.home, pathVar),
  };
}
