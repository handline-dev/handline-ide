import { DaemonClient, RpcError, redeemPairing, webSocketTransport, type Transport } from '@handline/client';
import type { KV } from '@handline/app-core';

const key = (url: string) => `token:${url}`;

/** The network listener serves its socket at /rpc; the Python daemon's own port has no path and takes no token. */
export const needsToken = (url: string): boolean => {
  try {
    return new URL(url).pathname === '/rpc';
  } catch {
    return false;
  }
};

export function loadToken(store: KV | null, url: string): string | null {
  try {
    return store?.getItem(key(url)) || null;
  } catch {
    return null;
  }
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
  const { token } = await redeemPairing(() => open(url), codeFrom(pasted), name);
  try {
    store?.setItem(key(url), token);
  } catch {
    /* blocked: paired for this run only */
  }
  return token;
}

export function forgetToken(store: KV | null, url: string): void {
  try {
    store?.setItem(key(url), ''); // KV has no remove; loadToken reads an empty one as none
  } catch {
    /* blocked */
  }
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
