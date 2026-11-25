import { DaemonClient, RpcError, redeemPairing, webSocketTransport, type Transport } from '@handline/client';
import type { KV } from '@handline/app-core';

const key = (url: string) => `token:${url}`;
// The device a token is for, by the address it was paired at; and per cluster (its master's node id) once the cluster named itself, with
// every door it listed, so another address of the same cluster needs no pairing. Only an address the cluster listed reuses its token.
const deviceKey = (url: string) => `device:${url}`;
const clusterKey = (master: string) => `cluster:${master}`;
const doorKey = (url: string) => `cluster-of:${url}`;

const read = (store: KV | null, k: string): string | null => {
  try {
    return store?.getItem(k) || null;
  } catch {
    return null;
  }
};
const write = (store: KV | null, k: string, v: string) => {
  try {
    store?.setItem(k, v);
  } catch {
    /* blocked: kept for this run only */
  }
};
type Held = { token: string; device: string | null; doors: string[] };
const heldBy = (store: KV | null, master: string | null): Held | null => {
  const held = master ? read(store, clusterKey(master)) : null;
  try {
    const h = held ? (JSON.parse(held) as Partial<Held>) : null;
    return h && typeof h.token === 'string'
      ? { token: h.token, device: typeof h.device === 'string' ? h.device : null, doors: Array.isArray(h.doors) ? h.doors : [] }
      : null;
  } catch {
    return null;
  }
};
/** What the cluster holds for `url`, only while its last door list names `url`: a door it dropped gets nothing. */
const clusterOf = (store: KV | null, url: string): Held | null => {
  const h = heldBy(store, read(store, doorKey(url)));
  return h?.doors.includes(url) ? h : null;
};

/** A connected cluster named its master and doors: what this address holds is kept for the cluster with that door list, replacing the
 *  last one, and each door points to it. */
export function rememberCluster(store: KV | null, url: string, master: string, doors: string[]): void {
  const was = clusterOf(store, url);
  const token = read(store, key(url)) ?? was?.token;
  if (!token) return;
  const list = [...new Set([url, ...doors])];
  write(store, clusterKey(master), JSON.stringify({ token, device: read(store, deviceKey(url)) ?? was?.device ?? null, doors: list }));
  for (const d of list) write(store, doorKey(d), master);
}

/** The device this client held for the daemon at `url` (its address's, else its cluster's), sent when it pairs again. */
export const heldDevice = (store: KV | null, url: string): string | null =>
  read(store, deviceKey(url)) ?? clusterOf(store, url)?.device ?? null;

/** The network listener serves its socket at /rpc; the Python daemon's own port has no path and takes no token. */
export const needsToken = (url: string): boolean => {
  try {
    return new URL(url).pathname === '/rpc';
  } catch {
    return false;
  }
};

/** The token for `url`: its own, else its cluster's when the cluster listed this address as one of its doors. */
export function loadToken(store: KV | null, url: string): string | null {
  return read(store, key(url)) ?? (clusterOf(store, url)?.token || null);
}

/** The code out of what the person pasted: the code `hl device pair` prints, or the link that holds it. */
export function codeFrom(text: string): string {
  const t = text.trim();
  const linked = invite(t.replace(/^[^#]*/, ''));
  if (linked) return linked.code;
  try {
    const c = new URL(t).searchParams.get('code');
    if (c) return c;
  } catch {
    /* a bare code */
  }
  return t;
}

export interface Invite {
  code: string;
  /** The listener's certificate fingerprint, for a door on a certificate of its own: the Mac app pins it. */
  fp: string | null;
}

/** What an invite link carries: the page opened as `/#pair=<code>`, `&fp=<fingerprint>` after it for a self-made certificate. Null for
 * any other hash. */
export function invite(hash: string): Invite | null {
  const m = /^#pair=([^&]+)(?:&fp=([0-9a-fA-F]{64}))?/.exec(hash);
  try {
    return m ? { code: decodeURIComponent(m[1]!), fp: m[2]?.toLowerCase() ?? null } : null;
  } catch {
    return null;
  }
}

/** The link Open in the Handline app opens: the same listener (`host` is host:port) and code, for the Mac app. */
export const appLink = (host: string, i: Invite): string =>
  `handline://pair?host=${encodeURIComponent(host)}&code=${encodeURIComponent(i.code)}${i.fp ? `&fp=${i.fp}` : ''}`;

/** What the device list calls this browser: the browser on the system, so the person can tell which one to revoke. */
export function deviceName(agent: string = navigator.userAgent): string {
  const browser = /Edg\//.test(agent)
    ? 'Edge'
    : /Firefox\//.test(agent)
      ? 'Firefox'
      : /Chrome\//.test(agent)
        ? 'Chrome'
        : /Safari\//.test(agent)
          ? 'Safari'
          : 'Browser';
  const system = /iPhone|iPad/.test(agent)
    ? 'iPhone'
    : /Android/.test(agent)
      ? 'Android'
      : /Mac OS/.test(agent)
        ? 'Mac'
        : /Windows/.test(agent)
          ? 'Windows'
          : /Linux/.test(agent)
            ? 'Linux'
            : '';
  return system ? `${browser} on ${system}` : browser;
}

/** Trades the pasted code for a device token and keeps it for this listener. Throws the daemon's own sentence on a refusal. */
export async function pairWith(
  url: string,
  pasted: string,
  store: KV | null,
  open: (url: string) => Promise<Transport> = webSocketTransport,
  name = 'Handline desktop',
): Promise<string> {
  const { token, device } = await redeemPairing(() => open(url), codeFrom(pasted), name, undefined, heldDevice(store, url) ?? undefined);
  write(store, key(url), token);
  write(store, deviceKey(url), device);
  return token;
}

/** A refused token goes, its address's and its cluster's when the cluster holds that same token; the device id stays, so pairing again
 *  replaces that device. */
export function forgetToken(store: KV | null, url: string): void {
  const refused = loadToken(store, url);
  const held = clusterOf(store, url);
  write(store, key(url), ''); // KV has no remove; an empty value reads as none
  if (held && held.token === refused) write(store, clusterKey(read(store, doorKey(url))!), JSON.stringify({ ...held, token: '' }));
  if (held?.device && !read(store, deviceKey(url))) write(store, deviceKey(url), held.device);
}

/** Whether the listener still knows this token: only its fixed `unpaired` refusal says no; a listener that cannot be reached says nothing. */
export async function tokenRefused(url: string, token: string, open: (url: string) => Promise<Transport>): Promise<boolean> {
  const c = new DaemonClient({ client: 'web-pc', token, connect: () => open(url) });
  try {
    await c.start();
    return false;
  } catch (e) {
    return e instanceof RpcError && e.gate === 'unpaired';
  } finally {
    c.close();
  }
}
