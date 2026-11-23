import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';

type Missing = { tool: string; fix: string; install?: { terminal: string; ended: boolean } };
const machine = { missing: [] as Missing[] };
const install = vi.fn(() => Promise.resolve({ tool: 'git', terminal: 'w1', host: 'tmux' }));
vi.mock('@handline/app-data', () => ({
  useStatus: () => ({ data: { missing_tools: machine.missing, vendors: [] }, refetch: vi.fn() }),
  useRead: () => ({}),
  useWrite: () => ({ mutateAsync: install }),
}));
vi.mock('#src/tools/TerminalTab.tsx', () => ({ PaneTab: ({ handle }: { handle: string }) => <i>pane {handle}</i> }));
import { NeedsGit } from '#src/install/install.tsx';

let root: Root;
let el: HTMLElement;
const show = (node: React.ReactNode) => act(async () => root.render(node));
const mount = async (node: React.ReactNode) => {
  el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
  await show(node);
};
const button = (text: string) => [...el.querySelectorAll('button')].find((b) => b.textContent === text);
const GIT = { tool: 'git', fix: 'xcode-select --install' };
afterEach(() => act(() => root.unmount()));

describe('NeedsGit', () => {
  it('shows the place as it is where git is installed', async () => {
    machine.missing = [{ tool: 'claude', fix: 'curl' }];
    await mount(<NeedsGit>the picker</NeedsGit>);
    expect(el.textContent).toBe('the picker');
  });
  it('says git is needed with Install, which asks the daemon to install it', async () => {
    machine.missing = [GIT];
    await mount(<NeedsGit>the picker</NeedsGit>);
    expect(el.textContent).toContain('git is needed for a project.');
    expect(el.textContent).not.toContain('the picker');
    await act(async () => button('Install git')!.click());
    expect(install).toHaveBeenCalledWith({ tool: 'git' });
  });
  it('while the daemon says it installs: one line instead of Install, its terminal a click away', async () => {
    machine.missing = [{ ...GIT, install: { terminal: 'w1', ended: false } }];
    await mount(<NeedsGit>the picker</NeedsGit>);
    expect(el.textContent).toContain('Installing git…');
    expect(button('Install git')).toBeUndefined();
    expect(el.textContent).not.toContain('pane w1');
    await act(async () => button('Details')!.click());
    expect(el.textContent).toContain('pane w1');
  });
  it('after an install that ended with git still missing: says so, with Install again', async () => {
    machine.missing = [{ ...GIT, install: { terminal: 'w1', ended: true } }];
    await mount(<NeedsGit>the picker</NeedsGit>);
    expect(el.textContent).toContain('git did not install.');
    expect(button('Install git')).toBeDefined();
  });
});
