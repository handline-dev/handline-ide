import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DaemonClient, highestMaster, memoryPair, type MasterSays, type Transport } from '@handline/client';
import { FakeDaemon } from '#src/daemon.ts';
import { until } from '#src/helpers.ts';

// Machines by url: a fake daemon each, or none for one that does not answer (a runner, which does not listen, or one switched off).
function machines(at: Record<string, MasterSays | null | 'gone'>) {
  const daemons = new Map<string, FakeDaemon>();
  for (const [url, m] of Object.entries(at)) {
    if (m === 'gone') continue;
    const d = new FakeDaemon();
    d.master = m;
    daemons.set(url, d);
  }
  const open = async (url: string): Promise<Transport> => {
    const d = daemons.get(url);
    if (!d) throw new Error(`${url} does not answer`);
    const [client, server] = memoryPair();
    d.accept(server);
    return client;
  };
  return { daemons, open };
}

test('the app joins the machine that answers as the highest master', async () => {
  const { open } = machines({
    a: { node: 'n-a', number: 1 },
    b: 'gone',
    c: { node: 'n-c', number: 2 },
    d: null,
  });
  const best = await highestMaster(['a', 'b', 'c', 'd'], open, undefined, 'web-pc');
  assert.deepEqual(best, { url: 'c', master: { node: 'n-c', number: 2 } });
});

test('at the same number the higher node id is master, as the daemon decides', async () => {
  const { open } = machines({ a: { node: 'n-b', number: 3 }, c: { node: 'n-a', number: 3 } });
  assert.equal((await highestMaster(['a', 'c'], open, undefined, 'web-pc'))?.url, 'a');
});

test('a master below the one seen is passed over in the search', async () => {
  const { open } = machines({ a: { node: 'n-a', number: 1 }, b: { node: 'n-b', number: 2 } });
  assert.equal(await highestMaster(['a', 'b'], open, undefined, 'web-pc', { node: 'n-c', number: 3 }), null);
});

test('no machine answering as master is null', async () => {
  const { open } = machines({ a: 'gone', b: null });
  assert.equal(await highestMaster(['a', 'b'], open, undefined, 'web-pc'), null);
});

test('a master below the one seen is refused, and the next connect is tried', async () => {
  const { open } = machines({ old: { node: 'n-old', number: 1 }, now: { node: 'n-now', number: 2 } });
  const order = ['old', 'now'];
  const tried: string[] = [];
  const below: MasterSays[] = [];
  const client = new DaemonClient({
    client: 'web-pc',
    connect: () => {
      const url = order[tried.length % order.length]!;
      tried.push(url);
      return open(url);
    },
    reconnect: { minMs: 1, maxMs: 2 },
    master: { floor: () => ({ node: 'n-now', number: 2 }), below: (m) => below.push(m) },
  });
  await assert.rejects(client.start(), /below the 2/);
  assert.deepEqual(below, [{ node: 'n-old', number: 1 }]);
  await client.start();
  assert.equal(client.hello?.capabilities.master?.node, 'n-now');
  assert.deepEqual(tried, ['old', 'now']);
  client.close();
});

test('a dropped master is left for the new one on the next connect, with the same client', async () => {
  const { daemons, open } = machines({ a: { node: 'n-a', number: 1 }, b: { node: 'n-b', number: 2 } });
  let at = 'a';
  const client = new DaemonClient({ client: 'web-pc', connect: () => open(at), reconnect: { minMs: 1, maxMs: 2 } });
  await client.start();
  at = 'b';
  daemons.get('a')!.dropConnections();
  await until(() => client.state === 'open' && client.hello?.capabilities.master?.node === 'n-b', 'on the new master');
  client.close();
});
