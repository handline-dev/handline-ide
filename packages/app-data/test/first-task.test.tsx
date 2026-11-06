import { afterEach, describe, expect, it, vi } from 'vitest';
import { setup } from '@handline/fake-daemon/helpers';
import { Store } from '@handline/app-core';
import { useCreateProgress } from '#src/index.ts';
import { mount, probe, unmount, until } from '#test/harness.tsx';

afterEach(unmount);

/** A new agent being made on the fake daemon, and the progress hook a screen mounts (again after a relaunch) on its operation. */
async function creating() {
  const s = setup();
  s.daemon.work.stepMs = 150;
  const store = new Store(s.client);
  await store.start();
  const { operation } = (await store.client.call('agent.create', {
    name: 'fresh',
    role: 'reviewer',
    repo: '/work/docs-site',
    base: 'master',
  } as never)) as { operation: string };
  const watch = async () => {
    const v = probe(() => useCreateProgress(operation, { task: 'Read the docs', box: '3h' }, undefined));
    await mount(store, v.node);
    return v;
  };
  const sent = (spy: { mock: { calls: unknown[][] } }) =>
    spy.mock.calls.filter((c) => c[0] === 'task.new').map((c) => c[1] as Record<string, unknown>);
  return { ...s, store, operation, watch, sent };
}

describe('the first task of a new agent', () => {
  it('is sent once, to the new agent, when its creation turns done', async () => {
    const { store, watch, sent } = await creating();
    const spy = vi.spyOn(store.client, 'call');
    const v = await watch();
    await until(() => sent(spy).length === 1, 'the task', 4000);
    expect(sent(spy)[0]).toMatchObject({ owner: 'fresh', title: 'Read the docs', time_box: '3h' });
    expect(v.get().done).toBe(true);
    await new Promise((r) => setTimeout(r, 200));
    expect(sent(spy)).toHaveLength(1);
  });

  it('is sent once under one request_id when the page is relaunched after a lost reply, and the daemon answers it as replayed', async () => {
    const { store, watch, sent, operation } = await creating();
    const real = store.client.call.bind(store.client) as (m: string, p: unknown) => Promise<unknown>;
    const replies: unknown[] = [];
    let lose = true;
    const spy = vi.spyOn(store.client, 'call').mockImplementation((async (method: string, p: unknown) => {
      const r = await real(method, p);
      if (method !== 'task.new') return r;
      replies.push(r);
      return lose ? new Promise(() => {}) : r;
    }) as never);
    await watch();
    await until(() => sent(spy).length === 1, 'the first send', 4000);
    lose = false;
    await unmount();
    await watch();
    await until(() => sent(spy).length === 2, 'the second send', 4000);
    expect(sent(spy).map((x) => x.request_id)).toEqual([`first-task:${operation}`, `first-task:${operation}`]);
    await until(() => replies.length === 2, 'the second reply');
    expect(replies[1]).toMatchObject({ replayed: true });
    const list = (await real('tasks.list', {})) as { items: { title: string }[] };
    expect(list.items.filter((t) => t.title === 'Read the docs')).toHaveLength(1);
  });
});
