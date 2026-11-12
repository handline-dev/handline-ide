import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';

const names: Record<string, string> = { 'a-7': 'web-settings', 'a-9': 'lander', 'a-2': 'architect' };
vi.mock('@handline/app-data', () => ({ useNameOf: () => (k: string) => names[k] ?? k }));
vi.mock('#src/settings/data/service.ts', () => ({ useService: () => null }));
vi.mock('#src/settings/data/machine.ts', () => ({
  useMachine: () => ({
    resources: null,
    agents: [{ name: 'a-7', cpu_pct: 12, rss_bytes: 1e8 }],
    leases: [
      {
        kind: 'simulator',
        max: 1,
        held: [{ lease: 1, agent: 'a-9', since: new Date().toISOString() }],
        queue: [{ position: 1, agent: 'a-2', says: '' }],
      },
    ],
    limits: { load_per_core_pct: 0, ram_free_min_pct: 0 },
    pending: null,
    stop: vi.fn(),
    setLimit: vi.fn(),
    setLeaseMax: vi.fn(),
    setAwake: vi.fn(),
    setRecord: vi.fn(),
    awake: 'off',
    record: false,
    working: 0,
  }),
}));
import { MachineTab } from '#src/settings/MachineTab.tsx';

let root: Root;
afterEach(() => act(() => root.unmount()));

describe('Settings > This Mac', () => {
  it('names the running agents and the holders and waiters of a simulator, not their keys', async () => {
    const el = document.createElement('div');
    document.body.append(el);
    root = createRoot(el);
    await act(async () => root.render(<MachineTab />));
    const text = el.textContent ?? '';
    for (const word of ['web-settings', 'lander', 'architect']) expect(text).toContain(word);
    for (const key of ['a-7', 'a-9', 'a-2']) expect(text).not.toContain(key);
    expect(el.querySelector('[aria-label="Stop web-settings"]')).not.toBeNull();
  });
});
