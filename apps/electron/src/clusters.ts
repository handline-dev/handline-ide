// The clusters this Mac joined (docs/story4-design.md section 1, slice 24). This Mac's own daemon keeps its board; for each cluster the
// shell runs an engine beside it: a handlined in a home of its own that joined that cluster's master as `hl device join` joins a
// machine (this person's Mac, on Off), so it runs that cluster's agents only once it is set to Run there. The page reaches each master
// itself, its certificate pinned, with the engine's device token; the shell keeps a connection to each to say whether it answers.
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import type { ShellCluster } from '@handline/app-core';
import type { DaemonClient } from '@handline/client';
import { joinLink, masterUrl, type Door, type KeptCluster, type OfferInput, type SettingsFile } from '#src/mode.ts';
import type { Joined } from '#src/joined.ts';

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
  /** What the engine's home joined (joined.ts joinedOf), null when it joined none. */
  joined(home: string): Joined | null;
  /** The engine's protected store, removed with its home on Leave. */
  store(home: string): string;
  engine(home: string): Engine;
  link(j: Joined): MasterLink;
  /** The list changed: a join, a leave, a master that answers or stops answering, a name learned. */
  changed(): void;
}

/** Where the engine of cluster `id` keeps its home: beside this Mac's own (~/.handline-clusters/<id>). */
export const engineHome = (home: string, id: string): string => path.join(`${path.resolve(home)}-clusters`, id);

interface Live {
  id: string;
  home: string;
  /** Read again each time the master answers: its master and the owner's machines change as it moves. */
  joined: Joined;
  engine: Engine;
  starting: Promise<void>;
  link: MasterLink;
}

export class Clusters {
  private live = new Map<string, Live>();
  private d: ClusterDeps;
  constructor(d: ClusterDeps) {
    this.d = d;
  }

  /** At launch: an engine and a connection for each cluster kept. A kept one whose home joined nothing is dropped. */
  start(): void {
    for (const c of this.kept()) {
      const home = engineHome(this.d.home, c.id);
      const j = this.d.joined(home);
      if (j) this.run(c.id, home, j);
      else this.keep(this.kept().filter((k) => k.id !== c.id));
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
    const home = engineHome(this.d.home, id);
    fs.mkdirSync(path.dirname(home), { recursive: true });
    fs.mkdirSync(home); // throws for an id taken already, whose home is not this join's to remove
    let j: Joined | null;
    try {
      await this.d.join(link, home);
      j = this.d.joined(home);
      if (!j) throw new Error('the join kept no credential');
    } catch (e) {
      this.remove(home);
      throw e;
    }
    this.keep([...this.kept(), { id, name: '' }]);
    return this.run(id, home, j);
  }

  /** Leaves a cluster: Off there, so its agents here hand over, then this Mac's device revoked, the engine stopped and its home
   * removed. A master that does not answer, or refuses, is no reason to stay: the device stays listed there until its owner revokes it. */
  async leave(id: string): Promise<void> {
    const l = this.live.get(id);
    if (!l) throw new Error('this Mac has not joined that cluster');
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
    // Still kept: the next launch runs it again, and a Leave then removes it.
    if (l.engine.state !== 'stopped') throw new Error(`its engine did not stop: ${l.engine.reason}`);
    l.engine.close();
    this.keep(this.kept().filter((k) => k.id !== id));
    this.remove(l.home);
  }

  private run(id: string, home: string, joined: Joined): ShellCluster {
    const engine = this.d.engine(home);
    const link = this.d.link(joined);
    const l: Live = { id, home, joined, engine, starting: engine.start(), link };
    this.live.set(id, l);
    link.onState((s) => {
      if (s === 'open') {
        l.joined = this.d.joined(home) ?? l.joined;
        this.learnName(id, link);
      }
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
