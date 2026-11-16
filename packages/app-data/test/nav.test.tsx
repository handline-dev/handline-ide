import { afterEach, describe, expect, it } from 'vitest';
import { second, setup } from '@handline/fake-daemon/helpers';
import { Store } from '@handline/app-core';
import { useNav } from '#src/index.ts';
import { mount, probe, unmount, until } from '#test/harness.tsx';

afterEach(unmount);
const ARCH = 'handline.architect';
const AUDITOR = 'handline.auditor';

async function connected() {
  const s = setup();
  const store = new Store(s.client);
  await store.start();
  return { ...s, store };
}

describe('the nav prefs', () => {
  it('start as the defaults, show a pin at once, and keep it once the daemon has it', async () => {
    const { store, daemon } = await connected();
    const p = probe(() => useNav());
    await mount(store, p.node);
    expect(p.get().nav.pins).toEqual([]);
    await until(() => p.get().ready, 'prefs served');
    p.get().togglePin(ARCH);
    await until(() => p.get().nav.pins.length === 1, 'the pin on screen');
    const kept = () => (daemon.admin.prefs.call('prefs.get', {}, () => ({})) as { nav: { pins: string[] } }).nav.pins;
    await until(() => kept().length === 1, 'the pin kept by the daemon');
    expect(kept()).toEqual([ARCH]);
    p.get().togglePin(AUDITOR);
    p.get().togglePin('handline.term-build');
    await until(() => kept().length === 3, 'both pins of one tick');
  });

  it("follow another device's change through the prefs event", async () => {
    const { store, daemon, net } = await connected();
    const p = probe(() => useNav());
    await mount(store, p.node);
    await until(() => p.get().ready, 'prefs served');
    const phone = new Store(second(daemon, net));
    await phone.start();
    await phone.client.call('prefs.set', { changes: { nav: { pins: [AUDITOR], layout: 'flat' } } });
    await until(() => p.get().nav.layout === 'flat', 'the change from the phone');
    expect(p.get().nav.pins).toEqual([AUDITOR]);
  });

  it('put a refused pin back', async () => {
    const { store } = await connected();
    const p = probe(() => useNav());
    await mount(store, p.node);
    await until(() => p.get().ready, 'prefs served');
    p.get().togglePin('nobody');
    await until(() => store.error !== null, 'the refusal');
    await until(() => p.get().nav.pins.length === 0, 'the pin taken back');
  });
});
