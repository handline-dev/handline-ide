import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';

const ws = {
  key: 'w1',
  name: 'handline',
  folder: '/w/handline',
  added_folders: ['/w/docs', '/w/notes'],
  lead: 'a1',
  folders: [
    { path: '/w/handline', name: 'handline', git: true, default: true },
    { path: '/w/docs', name: 'docs', git: true, default: false },
    { path: '/w/notes', name: 'notes', git: false, default: false },
  ],
};
const mutate = vi.fn();
vi.mock('@handline/app-data', () => ({
  useNameOf: () => (k: string) => k,
  useWrite: () => ({ mutate, isPending: false, error: null }),
  useAgents: () => ({ data: { agents: [{ key: 'a1', workspace: 'w1', folder: null }] } }),
  classify: (e: unknown) => e,
  sentence: String,
  hooks: { useWorkspacesList: () => ({ data: { workspaces: [ws] } }) },
}));
vi.mock('#src/shell.ts', () => ({ shell: () => null }));

const { AddFolder } = await import('#src/WorkspaceFolders.tsx');

describe('Add folder', () => {
  it('without a folder chooser takes a typed path and adds it to the workspace', () => {
    const el = document.createElement('div');
    const root = createRoot(el);
    act(() => root.render(<AddFolder wsKey="w1" />));
    act(() => el.querySelector('button')!.click());
    const input = el.querySelector('input')!;
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, '/w/more');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => [...el.querySelectorAll('button')].find((b) => b.textContent === 'Add')!.click());
    expect(mutate).toHaveBeenCalledWith({ workspace: 'w1', add_folder: '/w/more' }, expect.anything());
    act(() => root.unmount());
  });
});
