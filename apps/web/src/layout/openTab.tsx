// Where the app opens a tab: here, beside (a split to the right, or the pane next to it when there is a split already) or below (a
// split down). What an element does to ask for one is openable.tsx.
import { createContext, useContext, type KeyboardEvent, type MouseEvent } from 'react';

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

const MAC = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);
/** The shortcut a menu shows beside a way to open. */
export const SHORTCUT: Partial<Record<Gesture, string>> = { tab: MAC ? '⌘ click' : 'Ctrl click', side: MAC ? '⌥ click' : 'Alt click' };

/** The pane a view sits in opens tabs next to itself; set by the workspace around each pane. */
export const PaneOpen = createContext<Open | null>(null);

// For callers outside any pane (the sidebar): the workspace on screen, set while it is mounted.
let shown: Open = () => {};
export const setShownWorkspace = (open: Open) => {
  shown = open;
};

/** Brings up an app page (Usage, Settings…) in place of what is on screen, as the sidebar's page items do; set once by the app. */
let toPage: (kind: string, label: string) => void = () => {};
export const setPageGo = (go: (kind: string, label: string) => void) => {
  toPage = go;
};
export const goPage = (kind: string, label: string) => toPage(kind, label);

/** Opens a tab: from the caller's own pane when it sits in one, else in the workspace on screen. */
export function useOpenTab(): Open {
  const pane = useContext(PaneOpen);
  return pane ?? ((opens, how) => shown(opens, how));
}
