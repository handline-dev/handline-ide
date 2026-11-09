// Where the app opens a tab: here, beside (a split to the right, or the pane next to it when there is a split already) or below (a
// split down). What an element does to ask for one is openable.tsx.
import { createContext, useContext, type KeyboardEvent, type MouseEvent } from 'react';

export type How = 'here' | 'side' | 'below';
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
