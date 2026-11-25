// What the app keeps in a small file in its data folder, readable by this user only: the owner's machines the page found, the clusters
// this Mac joined, whether it opens at login, and whether quitting asks first. Also the link a Mac joins a cluster with and what the
// tray says about the daemon.
import fs from 'node:fs';
import path from 'node:path';
import { mismatch } from '@handline/app-core';
import { isChannel, type UpdateChannel } from '#src/update-channel.ts';
import { isTailscaleHost, listenerUrl, parsePairingUri, type PairingOffer } from '@handline/client';

/** The master a runner Mac obeys: where it is, which certificate to trust (null: a plain door), this machine's device token there. */
export interface Master {
  host: string;
  fingerprint: string | null;
  token: string;
  device: string;
}

/** One of the owner's machines, where it serves as master: host:port and its certificate's sha256 (hex), '' for a plain door. */
export interface Door {
  addr: string;
  fp: string;
}

const MAX_DOORS = 64;
const HEX64 = /^[0-9a-f]{64}$/;
const LOOPBACK = ['127.0.0.1', 'localhost', '[::1]'];
/** The doors the page handed over, when they are that: at most 64, each a host:port and a fingerprint. */
export const doorsOf = (v: unknown): Door[] | null =>
  Array.isArray(v) && v.length <= MAX_DOORS && v.every(isDoor) ? v.map((d: Door) => ({ addr: d.addr, fp: d.fp })) : null;

/** A plain door (no fingerprint) only where the link is already private: loopback or Tailscale. */
const isDoor = (d: Partial<Door> | null): d is Door => {
  if (!d || typeof d.addr !== 'string' || d.addr.length > 256 || !/:\d+$/.test(d.addr) || typeof d.fp !== 'string') return false;
  const host = d.addr.replace(/:\d+$/, '');
  return HEX64.test(d.fp) || (d.fp === '' && (LOOPBACK.includes(host) || isTailscaleHost(host)));
};

export interface Settings {
  /** The owner's machines the page found (node.status), trusted by fingerprint so the page can join whichever is master next. */
  doors?: Door[];
  /** Open Handline at login, in the tray only. On unless turned off. */
  openAtLogin: boolean;
  /** The release channel updates come from, when the person picked one; else the one the build was made for (update-channel.ts). */
  updateChannel?: UpdateChannel;
  /** Quit without asking first ("Don't show again"). */
  quietQuit?: boolean;
  /** The clusters this Mac joined (clusters.ts): each engine's home is named by its id; the name is the one its master last gave. */
  clusters?: KeptCluster[];
}

export interface KeptCluster {
  id: string;
  name: string;
  /** The port its engine listens on, to serve as that cluster's master once made it. */
  port?: number;
}

const ID = /^[0-9a-f]{8}$/;
const clustersOf = (v: unknown): KeptCluster[] | null =>
  Array.isArray(v)
    ? v.flatMap((c: Partial<KeptCluster> | null) =>
        c && typeof c.id === 'string' && ID.test(c.id)
          ? [
              {
                id: c.id,
                name: typeof c.name === 'string' ? c.name.slice(0, 200) : '',
                ...(typeof c.port === 'number' && Number.isInteger(c.port) && c.port > 0 && c.port < 65536 ? { port: c.port } : {}),
              },
            ]
          : [],
      )
    : null;

export class SettingsFile {
  private file: string;
  constructor(file: string) {
    this.file = file;
  }
  /** What is kept, or the defaults. */
  read(): Settings {
    let kept: Partial<Settings> = {};
    try {
      kept = JSON.parse(fs.readFileSync(this.file, 'utf8')) as Partial<Settings>;
    } catch {
      /* none yet */
    }
    const doors = doorsOf(kept.doors);
    const clusters = clustersOf(kept.clusters);
    return {
      openAtLogin: kept.openAtLogin !== false,
      ...(isChannel(kept.updateChannel) ? { updateChannel: kept.updateChannel } : {}),
      ...(kept.quietQuit === true ? { quietQuit: true } : {}),
      ...(doors ? { doors } : {}),
      ...(clusters?.length ? { clusters } : {}),
    };
  }
  write(s: Settings): void {
    fs.mkdirSync(path.dirname(this.file), { recursive: true, mode: 0o700 });
    fs.writeFileSync(this.file, JSON.stringify(s), { mode: 0o600 });
  }
}

/** What the person gave for the master: the link its Connect a device shows, the `handline://pair` link the link page opens the app with,
 * or its address and the code. */
export interface OfferInput {
  link?: string;
  address?: string;
  code?: string;
}

export function offerFrom(i: OfferInput): PairingOffer {
  const link = i.link?.trim();
  const web = link && /^https?:\/\/([^/#?]+)\/?#pair=([^&]+)(?:&fp=([0-9a-fA-F]{64}))?$/.exec(link); // Copy link's invite
  if (web) return offerOf(`handline://pair?host=${encodeURIComponent(web[1]!)}&code=${web[2]}${web[3] ? `&fp=${web[3]}` : ''}`);
  if (link) return offerOf(link);
  const address = i.address
    ?.trim()
    .replace(/^[a-z]+:\/\//, '')
    .replace(/\/.*$/, '');
  const code = i.code?.trim();
  if (!address || !code) throw new Error('give the pairing link, or the address and the code');
  const host = /:\d+$/.test(address) ? address : `${address}:7443`;
  return offerOf(`handline://pair?host=${encodeURIComponent(host)}&code=${encodeURIComponent(code)}`);
}

/** A link without a certificate fingerprint is trusted only for a Tailscale name (parsePairingUri) or, as a plain door is (isDoor),
 * this machine's loopback, where the link is already private. */
function offerOf(uri: string): PairingOffer {
  const q = URL.canParse(uri) ? new URL(uri).searchParams : null;
  const host = q?.get('host') ?? '';
  const code = q?.get('code');
  if (code && !q?.get('fp') && LOOPBACK.includes(host.replace(/:\d+$/, ''))) return { host, fingerprint: null, code };
  return parsePairingUri(uri);
}

/** The link `hl device join` takes, from what the person gave; a bad link or an untrusted address throws its own sentence. */
export function joinLink(i: OfferInput): string {
  const o = offerFrom(i);
  const fp = o.fingerprint ? `&fp=${o.fingerprint}` : '';
  return `handline://pair?host=${encodeURIComponent(o.host)}&code=${encodeURIComponent(o.code)}${fp}`;
}

/** Where the window reaches the master. */
export const masterUrl = (m: Master): string => listenerUrl(m.host, m.fingerprint !== null);

export type LinkStatus = 'connecting' | 'connected' | 'no-daemon';

/** What the tray says about the daemon the app reaches: which one (address, version) and whether it answers. */
export class Link {
  status: LinkStatus = 'connecting';
  address: string | null = null;
  version: string | null = null;
  /** The daemon's version as hello gave it, and the build the app is (build.json): when they differ the tray says so. */
  daemon: string | null = null;
  app: string | null = null;
  reason = '';
  private listeners: (() => void)[] = [];
  onChange(f: () => void): void {
    this.listeners.push(f);
  }
  set(status: LinkStatus, p: { address?: string | null; reason?: string } = {}): void {
    this.status = status;
    if (p.address !== undefined) this.address = p.address;
    this.reason = p.reason ?? '';
    for (const f of this.listeners) f();
  }
  /** 'Daemon is x, app is y' while the two are not the same build; null otherwise. */
  mismatch(): string | null {
    return mismatch(this.daemon, this.app);
  }
  /** The daemon's version: the loaded git sha its hello gives, short. */
  setVersion(daemon: string | undefined): void {
    this.version = daemon ? daemon.slice(0, 7) : null;
    this.daemon = daemon ?? null;
    for (const f of this.listeners) f();
  }
}
