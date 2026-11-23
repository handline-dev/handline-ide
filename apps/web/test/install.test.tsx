import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';

const ok = (data: unknown) => ({ isSuccess: true, isPending: false, data });
const machine = { missing: [] as { tool: string; fix: string }[] };
vi.mock('@handline/app-data', () => ({
  useStatus: () => ok({ missing_tools: machine.missing, vendors: [] }),
  useRead: () => ok({}),
  useWrite: () => ({ mutate: vi.fn(), error: null }),
  classify: (e: unknown) => e,
  sentence: String,
}));
const opened = vi.fn();
vi.mock('#src/ui/modal.tsx', () => ({ openModal: (...a: unknown[]) => (opened(...a), Promise.resolve()), ModalFrame: () => null }));
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
  it('says git is needed, with Install that opens the install pane, where git is missing', async () => {
    machine.missing = [{ tool: 'git', fix: 'xcode-select --install' }];
    await mount();
    expect(el.textContent).toContain('git is needed for a project.');
    expect(el.textContent).not.toContain('the picker');
    await act(async () => [...el.querySelectorAll('button')].find((b) => b.textContent === 'Install git')!.click());
    expect(opened).toHaveBeenCalledTimes(1);
  });
});
