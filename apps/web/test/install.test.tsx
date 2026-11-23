import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';

const ok = (data: unknown) => ({ isSuccess: true, isPending: false, data });
const machine = { missing: [] as { tool: string; fix: string }[] };
vi.mock('@handline/app-data', () => ({
  useStatus: () => ok({ missing_tools: machine.missing, vendors: [] }),
  useRead: () => ok({}),
  useWrite: (method: string) => ({ mutate: vi.fn(), mutateAsync: method === 'tools.install' ? install : vi.fn(), error: null }),
  classify: (e: unknown) => e,
  sentence: String,
}));
const install = vi.fn(() => Promise.resolve({ tool: 'git', terminal: 'w1', host: 'tmux' }));
vi.mock('#src/tools/TerminalTab.tsx', () => ({ PaneTab: ({ handle }: { handle: string }) => <i>pane {handle}</i> }));
import { NeedsGit } from '#src/install/install.tsx';

let root: Root;
let el: HTMLElement;
const mount = async () => {
  el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
  await act(async () => root.render(<NeedsGit>the picker</NeedsGit>));
};
afterEach(() => act(() => root.unmount()));

describe('NeedsGit', () => {
  it('shows the place as it is where git is installed', async () => {
    machine.missing = [{ tool: 'claude', fix: 'curl' }];
    await mount();
    expect(el.textContent).toBe('the picker');
  });
  it('says git is needed with Install, which turns into one line saying it installs, its terminal a click away', async () => {
    machine.missing = [{ tool: 'git', fix: 'xcode-select --install' }];
    await mount();
    expect(el.textContent).toContain('git is needed for a project.');
    expect(el.textContent).not.toContain('the picker');
    const button = (text: string) => [...el.querySelectorAll('button')].find((b) => b.textContent === text)!;
    await act(async () => button('Install git').click());
    expect(install).toHaveBeenCalledWith({ tool: 'git' });
    expect(el.textContent).toContain('Installing git…');
    expect(el.textContent).not.toContain('Install git');
    expect(el.textContent).not.toContain('pane w1');
    await act(async () => button('Details').click());
    expect(el.textContent).toContain('pane w1');
  });
});
