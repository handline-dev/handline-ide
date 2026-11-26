import { MAC } from '#src/ui/keys.ts';
// Where the app opens a tab: here, beside (a split to the right, or the pane next to it when there is a split already) or below (a
// split down). What an element does to ask for one is openable.tsx.
import { createContext, useContext, useEffect, useRef, type KeyboardEvent, type MouseEvent } from 'react';

export type How = 'here' | 'side' | 'below';
export type Open = (opens: string, how?: How) => void;

/** How a click or key asks to open; 'here' is what a plain click does where it is. */
export type Gesture = How | 'tab';

/** The app's one scheme: Cmd (Ctrl off the Mac) or a middle click opens a new tab, Option (Alt) beside; else here. Below is only
 * in the menu. */
export const howOf = (e: MouseEvent | KeyboardEvent): Gesture =>
  e.metaKey || e.ctrlKey || ('button' in e && e.button === 1) ? 'tab' : e.altKey ? 'side' : 'here';

/** In a pane a new tab is a plain open. */
export const inPane = (g: Gesture): How => (g === 'tab' ? 'here' : g);

/** The shortcut a menu shows beside a way to open. */
export const SHORTCUT: Partial<Record<Gesture, string>> = { tab: MAC ? '⌘ click' : 'Ctrl click', side: MAC ? '⌥ click' : 'Alt click' };

/** The pane a view sits in opens tabs next to itself; set by the workspace around each pane. */
export const PaneOpen = createContext<Open | null>(null);

// For callers outside any pane (the sidebar): the workspace on screen, set while it is mounted; a layer over it (the town) takes
// them while it is up.
let shown: Open = () => {};
export const setShownWorkspace = (open: Open) => {
  shown = open;
};
let over: Open[] = [];
/** While mounted, what opens from outside a pane opens through `open` (the latest given), not in the workspace under it. */
export function useOpensOver(open: Open) {
  const latest = useRef(open);
  useEffect(() => {
    latest.current = open;
  });
  useEffect(() => {
    const mine: Open = (opens, how) => latest.current(opens, how);
    over = [...over, mine];
    return () => {
      over = over.filter((o) => o !== mine);
    };
  }, []);
}

/** Brings up an app page (Usage, Settings…) in place of what is on screen, as the sidebar's page items do; set once by the app. */
let toPage: (kind: string, label: string) => void = () => {};
export const setPageGo = (go: (kind: string, label: string) => void) => {
  toPage = go;
};
export const goPage = (kind: string, label: string) => toPage(kind, label);

/** Brings up an agent's workspace, as its sidebar row does; set once by the app. */
let toAgent: (key: string) => void = () => {};
export const setAgentGo = (go: (key: string) => void) => {
  toAgent = go;
};
export const goAgent = (key: string) => toAgent(key);

/** Opens a tab: from the caller's own pane when it sits in one, else in the workspace on screen. */
export function useOpenTab(): Open {
  const pane = useContext(PaneOpen);
  return pane ?? ((opens, how) => (over[over.length - 1] ?? shown)(opens, how));
}
