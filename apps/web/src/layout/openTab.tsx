// How the app opens a tab, in one place. A click opens it here; Cmd+click (Ctrl+click off the Mac), a middle click or Cmd+Enter
// opens it beside, in a split to the right (the pane next to it when there is a split already), as an editor's "open to the side".
// Shift and Alt are kept free for later ways (split down, open without going there).
import { createContext, useContext, type KeyboardEvent, type MouseEvent } from 'react';

export type How = 'here' | 'side';
export type Open = (opens: string, how?: How) => void;

/** Which way an event asks for: beside with Cmd or Ctrl held, or a middle click; else here. */
export const howOf = (e: MouseEvent | KeyboardEvent): How =>
  e.metaKey || e.ctrlKey || ('button' in e && e.button === 1) ? 'side' : 'here';

/** The pane a view sits in opens tabs next to itself; set by the workspace around each pane. */
export const PaneOpen = createContext<Open | null>(null);

// For callers outside any pane (the sidebar): the workspace on screen, set while it is mounted.
let shown: Open = () => {};
export const setShownWorkspace = (open: Open) => {
  shown = open;
};

/** Opens a tab: from the caller's own pane when it sits in one, else in the workspace on screen. */
export function useOpenTab(): Open {
  const pane = useContext(PaneOpen);
  return pane ?? ((opens, how) => shown(opens, how));
}

/** The props that make an element open something the app's way: click, Cmd/Ctrl+click, middle click, Enter and Cmd+Enter.
 * Plain Enter is left to the element's own click (a button's), so it does not open twice. */
export function openProps(open: (how: How) => void) {
  return {
    onClick: (e: MouseEvent) => open(howOf(e)),
    onAuxClick: (e: MouseEvent) => {
      if (e.button === 1) open('side');
    },
    onKeyDown: (e: KeyboardEvent) => {
      if (e.key !== 'Enter' || !(e.metaKey || e.ctrlKey)) return;
      e.preventDefault();
      open('side');
    },
  };
}
