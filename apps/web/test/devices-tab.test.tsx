import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ModalHost } from '#src/ui/modal.tsx';

const NOW = Date.now();
const day = 86_400_000;
const dev = (id: string, o: object) => ({
  id,
  name: id,
  role: 'owner',
  created: new Date(NOW - 60 * day).toISOString(),
  push: false,
  last_seen: null,
  connected: false,
  current: false,
  stale: false,
  ...o,
});
const list = [
  dev('old-phone', { last_seen: new Date(NOW - 40 * day).toISOString(), stale: true }),
  dev('laptop', { connected: true }),
  dev('this-browser', { connected: true, current: true }),
];
let revoke: (id: string) => Promise<void>;
vi.mock('#src/settings/data/devices.ts', () => ({
  useDevices: () => ({ loading: false, error: false, devices: list, pushPlatforms: [], pending: null, revoke: (id: string) => revoke(id) }),
}));
vi.mock('#src/settings/PairSheet.tsx', () => ({ openPairSheet: vi.fn() }));
import { DevicesTab, sortDevices } from '#src/settings/DevicesTab.tsx';

let root: Root;
beforeEach(async () => {
  const el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
  await act(async () =>
    root.render(
      <>
        <DevicesTab />
        <ModalHost />
      </>,
    ),
  );
});
afterEach(async () => {
  await act(async () => root.unmount());
  document.body.innerHTML = '';
});
const flush = () => act(async () => new Promise((r) => setTimeout(r, 20)));

describe('Devices', () => {
  it('lists this device first, then connected, then the rest', () => {
    expect(sortDevices(list as never).map((d) => d.id)).toEqual(['this-browser', 'laptop', 'old-phone']);
    const rows = [...document.querySelectorAll('[data-device]')].map((r) => r.getAttribute('data-device'));
    expect(rows).toEqual(['this-browser', 'laptop']);
  });

  it('folds devices not seen for over 30 days until opened, and revokes them together', async () => {
    expect(document.querySelector('[data-device="old-phone"]')).toBeNull();
    const fold = [...document.querySelectorAll('button')].find((b) => b.textContent?.includes('not seen for over 30 days'))!;
    await act(async () => fold.click());
    expect(document.querySelector('[data-device="old-phone"]')!.textContent).toContain('Seen 40d ago');
    expect([...document.querySelectorAll('button')].some((b) => b.textContent === 'Revoke all 1')).toBe(true);
  });

  it('marks this device and who is connected', () => {
    expect(document.querySelector('[data-device="this-browser"]')!.textContent).toContain('This device');
    expect(document.querySelector('[data-device="laptop"]')!.textContent).toContain('Connected');
  });

  it('keeps the revoke confirm open, dimmed, until the daemon answers', async () => {
    let answer!: () => void;
    revoke = () => new Promise<void>((r) => (answer = r));
    await act(async () => document.querySelector<HTMLButtonElement>('[aria-label="Revoke laptop"]')!.click());
    await flush();
    const ok = [...document.querySelectorAll('button')].reverse().find((b) => b.textContent === 'Revoke')!;
    await act(async () => ok.click());
    expect([...document.querySelectorAll('button')].find((b) => b.textContent === 'Revoking…')!.disabled).toBe(true);
    await act(async () => answer());
    await flush();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it('says why when the daemon refuses', async () => {
    revoke = () => Promise.reject('No such device.');
    await act(async () => document.querySelector<HTMLButtonElement>('[aria-label="Revoke laptop"]')!.click());
    await flush();
    const ok = [...document.querySelectorAll('button')].reverse().find((b) => b.textContent === 'Revoke')!;
    await act(async () => ok.click());
    await flush();
    expect(document.querySelector('[role="alert"]')!.textContent).toBe('No such device.');
  });
});
