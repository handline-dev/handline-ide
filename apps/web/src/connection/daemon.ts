import type { KV } from '@handline/app-core';

const LOOPBACK = new Set(['localhost', '127.0.0.1', '[::1]', '::1']);

/**
 * The daemon's WebSocket: `?daemon=`, else `/rpc` on the page's own origin - the daemon that served a built page,
 * or in development the vite server's tunnel to the daemon (HANDLINE_LISTEN_WS). Plain ws is for loopback or the
 * page's own origin only; a daemon listener off loopback should be served over https, or its token travels in clear.
 */
export function daemonUrl(search: string, loc: { protocol: string; host: string }): { url: string; insecure: boolean } {
  const ws = loc.protocol === 'https:' ? 'wss' : 'ws';
  const given = new URLSearchParams(search).get('daemon');
  if (given === null) return { url: `${ws}://${loc.host}/rpc`, insecure: false };
  let insecure = true;
  try {
    const u = new URL(given);
    insecure = u.protocol !== 'wss:' && !LOOPBACK.has(u.hostname) && u.host !== loc.host;
  } catch {
    /* not a URL */
  }
  return { url: given, insecure };
}

export function browserStorage(): KV | null {
  try {
    return localStorage;
  } catch {
    return null;
  }
}
