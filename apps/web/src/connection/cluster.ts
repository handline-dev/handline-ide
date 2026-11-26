// The owner's machines, kept so the page finds the master after it moves (docs/several-machines.md section 4): each machine's door from
// node.status, the master last joined and the highest master number seen. The device token stays the one this browser paired with:
// the board's copy carries it to the new master.
import type { KV } from '@handline/app-core';
import { above, highestMaster, isTailscaleHost, type MasterSays, type Node, type Transport } from '@handline/client';

export type Kept = {
  /** Each machine's door, as a WebSocket url. */
  urls: string[];
  /** The url the page last joined a master at. */
  last: string | null;
  /** The highest master seen, and its machine's name. */
  master: (MasterSays & { name: string }) | null;
};

const KEY = 'machines';
const LOOPBACK = new Set(['localhost', '127.0.0.1', '[::1]']);

/** What is kept for the machines the page's url belongs to; one browser may know more than one owner's machines. */
export function keptFor(store: KV | null, url: string): Kept {
  const all = readAll(store);
  return all.find((k) => k.urls.includes(url) || k.last === url) ?? { urls: [url], last: null, master: null };
}

export function keep(store: KV | null, url: string, k: Kept): void {
  const rest = readAll(store).filter((o) => !o.urls.includes(url) && o.last !== url && !o.urls.some((u) => k.urls.includes(u)));
  try {
    store?.setItem(KEY, JSON.stringify([k, ...rest]));
  } catch {
    /* blocked: found again this run only */
  }
}

function readAll(store: KV | null): Kept[] {
  try {
    const v: unknown = JSON.parse(store?.getItem(KEY) || '[]');
    return Array.isArray(v) ? v.filter(isKept) : [];
  } catch {
    return [];
  }
}

const isKept = (v: unknown): v is Kept => {
  const k = v as Partial<Kept> | null;
  return !!k && Array.isArray(k.urls) && k.urls.every((u) => typeof u === 'string') && (k.last === null || typeof k.last === 'string');
};

/** A door's url: wss with a certificate, plain ws only where the link is already private (loopback, Tailscale); null otherwise. */
export function doorUrl(door: { addr: string; fp: string }): string | null {
  if (door.fp) return `wss://${door.addr}/rpc`;
  const host = door.addr.replace(/:\d+$/, '');
  return LOOPBACK.has(host) || isTailscaleHost(host) ? `ws://${door.addr}/rpc` : null;
}

/** The kept machines after a node.status: every door it lists, the page's own url first. */
export function withNodes(k: Kept, url: string, nodes: Node[]): Kept {
  const doors = nodes.flatMap((n) => (n.door ? [doorUrl(n.door)] : [])).filter((u): u is string => !!u);
  return { ...k, urls: [...new Set([url, ...doors])] };
}

/** The highest master of the two; a seen one keeps its name when the hello names none. */
export function higher(seen: Kept['master'], said: MasterSays, name?: string): NonNullable<Kept['master']> {
  if (seen && !above(said, seen)) return said.node === seen.node && name ? { ...seen, name } : seen;
  return { ...said, name: name ?? (seen?.node === said.node ? seen.name : '') };
}

/** The page's connect: the master it last joined, else the kept machine that answers as the highest master not below the one seen. */
export function masterConnect(o: { url: string; kept: () => Kept; open: (url: string) => Promise<Transport>; token: string | undefined }) {
  let at = o.kept().last ?? o.url;
  let refused: string | null = null;
  return {
    at: () => at,
    below: () => void (refused = at),
    connect: async (): Promise<Transport> => {
      const skip = refused;
      refused = null;
      if (skip !== at) {
        try {
          return await o.open(at);
        } catch (e) {
          const found = await look(at);
          if (!found) throw e;
        }
      } else if (!(await look(skip))) throw new Error('no machine answers as master');
      return o.open(at);
    },
  };
  async function look(past: string): Promise<boolean> {
    const others = o.kept().urls.filter((u) => u !== past);
    const best = others.length ? await highestMaster(others, o.open, o.token, 'web-pc', o.kept().master) : null;
    if (best) at = best.url;
    return !!best;
  }
}

/** The kept machines of one page: read by its connect at each try, changed by hello and node.status, saved as they change. */
export class Machines {
  private k: Kept;
  private listeners = new Set<() => void>();
  private store: KV | null;
  readonly url: string;
  constructor(store: KV | null, url: string) {
    this.store = store;
    this.url = url;
    this.k = keptFor(store, url);
  }
  get = (): Kept => this.k;
  subscribe = (f: () => void): (() => void) => (this.listeners.add(f), () => void this.listeners.delete(f));
  private set(k: Kept): void {
    this.k = k;
    keep(this.store, this.url, k);
    for (const f of this.listeners) f();
  }
  /** A hello at `at` named this master. */
  joined(at: string, said: MasterSays): void {
    this.set({ ...this.k, last: at, master: higher(this.k.master, said) });
  }
  /** node.status: every door, and the master's name. */
  seen(st: { master: { id: string; number: number }; nodes: Node[] }): void {
    const k = withNodes(this.k, this.url, st.nodes);
    const name = st.nodes.find((n) => n.id === st.master.id)?.name;
    this.set({ ...k, master: higher(k.master, { node: st.master.id, number: st.master.number }, name) });
  }
}
