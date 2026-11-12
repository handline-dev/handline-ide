import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';

// The daemon's names for the keys in its text: a lead reads as its workspace's name.
vi.mock('@handline/app-data', () => ({
  useDirectory: () => ({
    agents: [
      { key: 'a1', name: 'lead', workspace: 'w1' },
      { key: 'a6', name: 'daemon-tasks', workspace: 'w1' },
    ],
    people: [],
    workspaces: [{ key: 'w1', name: 'handline', folder: '/h', added_folders: [], lead: 'a1' }],
  }),
}));

import { Refs } from '#src/ui/refs.tsx';
import { toast, ToastHost } from '#src/ui/toast.tsx';

function render(node: React.ReactNode) {
  const container = document.createElement('div');
  act(() => createRoot(container).render(node));
  return container;
}

describe('daemon text with <@key> tokens', () => {
  it('reads as names in a plain-text spot', () => {
    const c = render(<Refs text="T9002: told <@a6>, no movement in 1h; ask <@a1>" />);
    expect(c.textContent).toBe('T9002: told @daemon-tasks, no movement in 1h; ask @handline');
    expect(c.textContent).not.toContain('<@');
  });
  it('reads as names in a toast given a string', () => {
    const c = render(<ToastHost />);
    act(() => void toast('The switch did not start: <@a1> holds it'));
    expect(c.textContent).toContain('The switch did not start: @handline holds it');
  });
});
