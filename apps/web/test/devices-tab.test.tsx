import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DeviceRow, Mode } from '#src/settings/data/devices.ts';
import { ModalHost } from '#src/ui/modal.tsx';

const NOW = Date.now();
const day = 86_400_000;
const row = (id: string, o: Partial<DeviceRow>): DeviceRow => ({
  id,
  name: id,
  kind: 'ios',
  runner: false,
  master: false,
  here: false,
  mine: true,
  mode: null,
  state: 'connected',
  since: null,
  agents: [],
  lacks: [],
  node: null,
  stale: false,
  ...o,
});
const rows = [
  row('mbp', { kind: 'mac', runner: true, master: true, here: true, mode: 'run', state: 'running', agents: ['ui', 'core'] }),
  row('studio', { kind: 'mac', runner: true, mode: 'run', state: 'running', lacks: [{ agent: 'ui', why: 'needs pencil' }] }),
  row('bob-mac', { kind: 'mac', runner: true, mine: false, mode: 'off', state: 'off' }),
  row('phone', {}),
  row('old-phone', { state: 'seen', since: new Date(NOW - 40 * day).toISOString(), stale: true }),
];
let revoke: (id: string) => Promise<void>;
const setMode = vi.fn((_r: DeviceRow, _m: Mode) => Promise.resolve());
vi.mock('#src/settings/data/devices.ts', async (orig) => ({
  ...(await orig<object>()),
  useDevices: () => ({ loading: false, error: false, rows, pushPlatforms: [], pending: null, setMode, revoke: (id: string) => revoke(id) }),
}));
vi.mock('@handline/app-data', () => ({ useNameOf: () => (k: string) => k }));
vi.mock('#src/settings/PairSheet.tsx', () => ({ openPairSheet: vi.fn() }));
vi.mock('#src/settings/MachineSections.tsx', () => ({ MachineSections: () => <p>this mac</p> }));
import { DevicesTab } from '#src/settings/DevicesTab.tsx';

let root: Root;
beforeEach(async () => {
  setMode.mockClear();
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
const at = (id: string) => document.querySelector<HTMLElement>(`[data-device="${id}"]`)!;
const button = (text: string) => [...document.querySelectorAll('button')].reverse().find((b) => b.textContent === text)!;
const radio = (id: string, word: string) =>
  [...at(id).querySelectorAll<HTMLButtonElement>('[role="radio"]')].find((b) => b.textContent === word)!;

describe('Devices', () => {
  it('lists every device in one list, the master first, with its state and what a Mac lacks', () => {
    expect([...document.querySelectorAll('[data-device]')].map((r) => r.getAttribute('data-device'))).toEqual([
      'mbp',
      'studio',
      'bob-mac',
      'phone',
    ]);
    expect(at('mbp').textContent).toContain('This device');
    expect(at('mbp').textContent).toContain('Master');
    expect(at('mbp').textContent).toContain('2 agents');
    expect(at('studio').textContent).toContain('cannot run ui: needs pencil');
    expect(at('phone').querySelector('[role="radiogroup"]')).toBeNull();
  });

  it('sets Run and Pause at once, asks before Off, and never lets the master go Off', async () => {
    await act(async () => radio('studio', 'Pause').click());
    expect(setMode).toHaveBeenCalledWith(expect.objectContaining({ id: 'studio' }), 'pause');
    await act(async () => radio('studio', 'Off').click());
    await flush();
    expect(setMode).toHaveBeenCalledTimes(1);
    await act(async () => button('Turn off').click());
    await flush();
    expect(setMode).toHaveBeenLastCalledWith(expect.objectContaining({ id: 'studio' }), 'off');
    expect(radio('mbp', 'Off').disabled).toBe(true);
  });

  it("lets only a Mac's own person set it", () => {
    expect(radio('bob-mac', 'Run').disabled).toBe(true);
  });

  it('acts on the picked devices together', async () => {
    await act(async () => at('studio').querySelector<HTMLInputElement>('input[type="checkbox"]')!.click());
    await act(async () => at('bob-mac').querySelector<HTMLInputElement>('input[type="checkbox"]')!.click());
    expect(document.querySelector('[aria-label="Selection"]')!.textContent).toContain('2 selected');
    const bar = document.querySelector('[aria-label="Selection"]')!;
    await act(async () => [...bar.querySelectorAll('button')].find((b) => b.textContent === 'Pause')!.click());
    // bob-mac is another person's: only studio is set.
    expect(setMode.mock.calls.map(([r, m]) => [r.id, m])).toEqual([['studio', 'pause']]);
  });

  it("opens a device's page from its row, and goes back", async () => {
    await act(async () => at('mbp').click());
    expect(document.body.textContent).toContain('this mac');
    await act(async () => button('Devices').click());
    expect(at('studio')).not.toBeNull();
  });

  it('folds devices not seen for over 30 days, and keeps the revoke confirm open, dimmed, until the daemon answers', async () => {
    expect(document.querySelector('[data-device="old-phone"]')).toBeNull();
    let answer!: () => void;
    revoke = () => new Promise<void>((r) => (answer = r));
    await act(async () => button('Revoke all 1').click());
    await flush();
    await act(async () => button('Revoke').click());
    expect(button('Revoking…').disabled).toBe(true);
    await act(async () => answer());
    await flush();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it('says why when the daemon refuses', async () => {
    revoke = () => Promise.reject('No such device.');
    await act(async () => button('Revoke all 1').click());
    await flush();
    await act(async () => button('Revoke').click());
    await flush();
    expect(document.querySelector('[role="alert"]')!.textContent).toBe('No such device.');
  });
});
