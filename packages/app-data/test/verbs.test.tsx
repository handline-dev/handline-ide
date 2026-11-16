import { act } from 'react';
import { afterEach, describe, it } from 'vitest';
import { setup } from '@handline/fake-daemon/helpers';
import { Store, VERBS } from '@handline/app-core';
import { useVerbRun } from '#src/index.ts';
import { mount, probe, unmount, until } from '#test/harness.tsx';

afterEach(unmount);
const ARCH = 'handline.architect';

async function connected() {
  const s = setup();
  const store = new Store(s.client);
  await store.start();
  return { ...s, store };
}

describe('the agent verb hooks, against the fake daemon', () => {
  it('runs a verb: the daemon has done it', async () => {
    const { store, daemon } = await connected();
    const v = probe(() => ({ run: useVerbRun(ARCH) }));
    await mount(store, v.node);
    act(() => v.get().run(VERBS.park));
    await until(() => daemon.sessions.get(ARCH)!.state === 'parked', 'the daemon');
  });

  it('says the refusal when the daemon refuses', async () => {
    const { store, daemon } = await connected();
    const v = probe(() => ({ run: useVerbRun(ARCH) }));
    await mount(store, v.node);
    daemon.knobs.refuse('agent.fold', 'state', 'it is mid-turn');
    act(() => v.get().run(VERBS.fold));
    await until(() => !!store.error?.includes('it is mid-turn'), 'the refusal');
  });

  it('sends the chosen account with Move account', async () => {
    const { store, daemon } = await connected();
    const v = probe(() => ({ run: useVerbRun(ARCH) }));
    await mount(store, v.node);
    act(() => v.get().run(VERBS.move, 'beta'));
    await until(() => daemon.sessions.get(ARCH)!.fx.account === 'beta', 'the daemon');
  });
});
