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
  behind: false,
  updating: false,
  appReady: '',
  ...o,
});
const rows = [
  row('mbp', { kind: 'mac', runner: true, master: true, here: true, mode: 'run', state: 'running', agents: ['ui', 'core'] }),
  row('studio', { kind: 'mac', runner: true, mode: 'run', state: 'running', lacks: [{ agent: 'ui', why: 'its project is not there' }] }),
  row('laptop', { kind: 'mac', runner: true, mode: 'run', state: 'running', behind: true }),
  row('mini', { kind: 'mac', runner: true, mode: 'run', state: 'running', appReady: '0.1.17' }),
  row('bob-mac', { kind: 'mac', runner: true, mine: false, mode: 'off', state: 'off' }),
  row('phone', {}),
  row('old-phone', { state: 'seen', since: new Date(NOW - 40 * day).toISOString(), stale: true }),
];
let revoke: (id: string) => Promise<void>;
const setMode = vi.fn((_r: DeviceRow, _m: Mode) => Promise.resolve());
const update = vi.fn((_r: DeviceRow) => Promise.resolve());
vi.mock('#src/settings/data/devices.ts', async (orig) => ({
  ...(await orig<object>()),
  useDevices: () => ({
    loading: false,
    error: false,
    rows,
    pushPlatforms: [],
    pending: null,
    setMode,
    update,
    revoke: (id: string) => revoke(id),
  }),
  useSetAside: () => aside.now,
}));
const aside = vi.hoisted(() => ({ now: null as { name: string; since: string; master: string; key: string } | null }));
vi.mock('@handline/app-data', async (real) => ({
  ...(await real<object>()),
  useNameOf: () => (k: string) => k,
  useServes: () => ({ served: true }),
}));
vi.mock('#src/settings/PairSheet.tsx', () => ({ openPairSheet: vi.fn() }));
vi.mock('#src/settings/MachineSections.tsx', () => ({ MachineSections: () => <p>this mac</p> }));
vi.mock('#src/settings/AgentPlacement.tsx', () => ({ AgentPlacement: () => null }));
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
  it('says when a board was set aside: which device was the master until when, and where the board is now', async () => {
    expect(document.querySelector('[role="note"]')).toBeNull();
    aside.now = { name: 'studio', since: '2026-11-21T11:23:00Z', master: 'mini', key: 'n@1' };
    await act(async () => root.unmount()); // a fresh page: the note is read as it opens
    root = createRoot(document.body.appendChild(document.createElement('div')));
    await act(async () => root.render(<DevicesTab />));
    expect(document.querySelector('[role="note"]')?.textContent).toMatch(
      /^studio was the master until .+\. Its board was set aside; the board is now on mini\.$/,
    );
    aside.now = null;
  });

  it('lists every device in one list, the master first, with what its buttons cannot say and what a Mac lacks', () => {
    expect([...document.querySelectorAll('[data-device]')].map((r) => r.getAttribute('data-device'))).toEqual([
      'mbp',
      'studio',
      'laptop',
      'mini',
      'bob-mac',
      'phone',
    ]);
    expect(at('mbp').textContent).toContain('This device');
    expect(at('mbp').textContent).toContain('Master');
    expect(at('mbp').textContent).toContain('2 agents');
    expect(at('mbp').textContent).not.toContain('Running');
    expect(at('mbp').querySelector('[aria-label="Mac"]')).not.toBeNull();
    expect(at('studio').querySelector('[data-lacks]')!.getAttribute('aria-label')).toBe('Cannot run ui: its project is not there.');
    expect(at('studio').textContent).not.toContain('Cannot run');
    expect(at('phone').querySelector('[role="radiogroup"]')).toBeNull();
    expect(at('phone').textContent).toContain('Controls only');
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
    expect(button('Revoke').disabled).toBe(true);
    expect(button('Revoke').getAttribute('aria-busy')).toBe('true');
    await act(async () => answer());
    await flush();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it('keeps Update out of the right-click', async () => {
    await act(async () => at('laptop').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, clientX: 5, clientY: 5 })));
    expect([...document.querySelectorAll('[role="menuitem"]')].map((m) => m.textContent)).not.toContain('Update');
    await act(async () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
  });

  it('says a Mac is on another build on its row, with its Update there', async () => {
    update.mockClear();
    expect(at('laptop').textContent).toContain('other build');
    const b = [...at('laptop').querySelectorAll('button')].find((x) => x.textContent === 'Update')!;
    await act(async () => b.click());
    expect(update).toHaveBeenCalledTimes(1);
    expect(document.querySelector('[data-device="studio"]')!.textContent).not.toContain('other build');
  });

  it("says a Mac's app has a newer version ready, as the Update its own person presses", async () => {
    update.mockClear();
    expect(at('mini').textContent).toContain('Update to 0.1.17');
    expect(at('mini').textContent).not.toContain('other build');
    const b = [...at('mini').querySelectorAll('button')].find((x) => x.textContent?.trim() === 'Update to 0.1.17')!;
    await act(async () => b.click());
    expect(update).toHaveBeenCalledTimes(1);
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
