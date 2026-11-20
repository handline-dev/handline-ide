import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';

const names: Record<string, string> = { 'a-7': 'web-settings', 'a-9': 'lander', 'a-2': 'architect' };
const run = {
  name: 'ios-build',
  scope: 'w1',
  cmds: ['xcodebuild'],
  at_once: 1,
  why: '',
  set_by: 'a-2',
  held: [{ agent: 'a-9', since: new Date().toISOString(), hold: false }],
  waiting: [{ agent: 'a-2', since: new Date().toISOString(), position: 1 }],
};
vi.mock('@handline/app-data', () => ({
  useNameOf: () => (k: string) => names[k] ?? k,
  useWrite: () => ({ mutate: vi.fn() }),
  classify: (e: unknown) => e,
  sentence: String,
  hooks: {
    useRunList: () => ({ data: { runs: [run], presets: [] } }),
    useWorkspacesList: () => ({ data: { workspaces: [{ key: 'w1', name: 'handline' }] } }),
  },
}));
vi.mock('#src/settings/data/service.ts', () => ({ useService: () => null }));
vi.mock('#src/settings/data/machine.ts', () => ({
  useMachine: () => ({
    resources: null,
    agents: [{ name: 'a-7', cpu_pct: 12, rss_bytes: 1e8 }],
    limits: { load_per_core_pct: 0, ram_free_min_pct: 0 },
    pending: null,
    stop: vi.fn(),
    setLimit: vi.fn(),
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
  it("names the running agents and a shared run's holders and waiters, not their keys, and its workspace by name", async () => {
    const el = document.createElement('div');
    document.body.append(el);
    root = createRoot(el);
    await act(async () => root.render(<MachineTab />));
    const text = el.textContent ?? '';
    for (const word of ['web-settings', 'lander', 'architect']) expect(text).toContain(word);
    for (const key of ['a-7', 'a-9', 'a-2']) expect(text).not.toContain(key);
    expect(text).toContain('handline');
    expect(text).not.toContain('w1');
    expect(el.querySelector('[aria-label="Stop web-settings"]')).not.toBeNull();
  });
});
