import { describe, expect, it } from 'vitest';
import { DaemonClient, memoryPair, PROTOCOL, type MasterSays, type Node, type Transport } from '@handline/client';
import type { KV } from '@handline/app-core';
import { doorUrl, higher, keep, keptFor, masterConnect, withNodes, type Kept } from '#src/connection/cluster.ts';

// Machines by url, each answering hello as the master it names (null: a daemon that names none); a url left out does not answer.
function machines(at: Record<string, MasterSays | null>) {
  const opened: string[] = [];
  const open = async (url: string): Promise<Transport> => {
    opened.push(url);
    if (!(url in at)) throw new Error(`${url} does not answer`);
    const [client, server] = memoryPair();
    server.onmessage = (line) => {
      const { id } = JSON.parse(line) as { id: number };
      const master = at[url];
      const result = {
        protocol: PROTOCOL,
        min_client: 1,
        role: 'owner',
        caller: { kind: 'device', key: 'owner' },
        methods: [],
        capabilities: master ? { master } : {},
      };
      server.send(JSON.stringify({ jsonrpc: '2.0', id, result }));
    };
    return client;
  };
  return { open, opened };
}

const memory = (): KV => {
  const m = new Map<string, string>();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => void m.set(k, v) };
};

const node = (id: string, door: Node['door'], extra: Partial<Node> = {}) => ({ id, name: id, door, ...extra }) as Node;

describe('the kept machines', () => {
  it('turn a door into a url: wss with a certificate, plain ws only on loopback or Tailscale', () => {
    expect(doorUrl({ addr: '10.0.0.2:7443', fp: 'ab' })).toBe('wss://10.0.0.2:7443/rpc');
    expect(doorUrl({ addr: '127.0.0.1:7462', fp: '' })).toBe('ws://127.0.0.1:7462/rpc');
    expect(doorUrl({ addr: 'mini.tail1.ts.net:7443', fp: '' })).toBe('ws://mini.tail1.ts.net:7443/rpc');
    expect(doorUrl({ addr: '10.0.0.2:7443', fp: '' })).toBeNull();
  });

  it('follow node.status and keep the page url first', () => {
    const k = withNodes({ urls: ['ws://page/rpc'], last: null, master: null }, 'ws://page/rpc', [
      node('a', { addr: '10.0.0.2:7443', fp: 'ab' }),
      node('b', null),
      node('c', { addr: '10.0.0.3:7443', fp: '' }),
    ]);
    expect(k.urls).toEqual(['ws://page/rpc', 'wss://10.0.0.2:7443/rpc']);
  });

  it('are found again from any of their urls', () => {
    const s = memory();
    const k: Kept = { urls: ['ws://a/rpc', 'ws://b/rpc'], last: 'ws://b/rpc', master: { node: 'b', number: 2, name: 'mini' } };
    keep(s, 'ws://a/rpc', k);
    expect(keptFor(s, 'ws://b/rpc')).toEqual(k);
    expect(keptFor(s, 'ws://c/rpc')).toEqual({ urls: ['ws://c/rpc'], last: null, master: null });
  });

  it('keep the highest master, and its name', () => {
    const seen = { node: 'a', number: 2, name: 'studio' };
    expect(higher(seen, { node: 'b', number: 1 })).toEqual(seen);
    expect(higher(seen, { node: 'b', number: 3 }, 'mini')).toEqual({ node: 'b', number: 3, name: 'mini' });
    expect(higher(seen, { node: 'a', number: 2 }, 'studio 2')).toEqual({ ...seen, name: 'studio 2' });
  });
});

describe('the page connect', () => {
  it('goes to the master it last joined, and, when that is gone, to the kept machine that is the highest master', async () => {
    const { open, opened } = machines({ b: { node: 'n-b', number: 2 }, c: { node: 'n-c', number: 3 }, d: null });
    const kept: Kept = { urls: ['a', 'b', 'c', 'd'], last: 'a', master: { node: 'n-a', number: 1, name: 'studio' } };
    const m = masterConnect({ url: 'a', kept: () => kept, open, token: 't' });
    const client = new DaemonClient({ client: 'web-pc', connect: m.connect, reconnect: false });
    await client.start();
    expect(m.at()).toBe('c');
    expect(client.hello?.capabilities.master).toEqual({ node: 'n-c', number: 3 });
    expect(opened[0]).toBe('a');
    client.close();
  });

  it('looks again, past it, after a master below the one seen answered', async () => {
    const { open } = machines({ old: { node: 'n-old', number: 1 }, now: { node: 'n-now', number: 2 } });
    const kept: Kept = { urls: ['old', 'now'], last: 'old', master: { node: 'n-now', number: 2, name: 'mini' } };
    const m = masterConnect({ url: 'old', kept: () => kept, open, token: 't' });
    const client = new DaemonClient({
      client: 'web-pc',
      connect: m.connect,
      reconnect: false,
      master: { floor: () => kept.master, below: m.below },
    });
    await expect(client.start()).rejects.toThrow(/below the 2/);
    await client.start();
    expect(m.at()).toBe('now');
    client.close();
  });

  it('fails with the first reason when no machine answers as master', async () => {
    const { open } = machines({ b: null });
    const m = masterConnect({ url: 'a', kept: () => ({ urls: ['a', 'b'], last: null, master: null }), open, token: 't' });
    await expect(m.connect()).rejects.toThrow('a does not answer');
  });
});
