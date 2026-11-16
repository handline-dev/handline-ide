// Which agents the agent list (every app) and the phone's dock show, from the person's prefs and this device's open tabs. Pure.
import type { AgentView, NavPrefs, Prefs, Worktree } from '@handline/client';

export const DEFAULT_NAV: NavPrefs = { pins: [], unread_only: false, hidden: [], layout: 'tree', order: [] };
/** How many open tabs, not counting pinned agents, the dock keeps. */
export const OPEN_MAX = 5;

export const navOf = (p: Prefs | undefined): NavPrefs => ({ ...DEFAULT_NAV, ...p?.nav });

/** The pinned agents in the person's order; a pin naming an agent that is gone is skipped. */
export function pinnedOf(agents: readonly AgentView[], pins: readonly string[]): AgentView[] {
  const byName = new Map(agents.map((a) => [a.key, a]));
  return pins.flatMap((n) => byName.get(n) ?? []);
}

/** An agent opened on this device, and the worktree it shows (absent: the one the agent owns). */
export interface OpenTab {
  agent: string;
  path?: string;
}
export interface DockItem {
  agent: AgentView;
  branch: string | null;
}

/** The branch of the worktree at `path`, else of the first one the agent owns. */
export const branchOf = (worktrees: readonly Worktree[], agent: string, path?: string): string | null =>
  worktrees.find((w) => (path ? w.path === path : w.owner === agent))?.branch ?? null;

/** The dock: the pinned agents, then the open tabs that are not pinned, each with its branch. */
export function navDock(
  agents: readonly AgentView[],
  pins: readonly string[],
  open: readonly OpenTab[],
  worktrees: readonly Worktree[],
): { pinned: DockItem[]; open: DockItem[] } {
  const item = (a: AgentView): DockItem => ({ agent: a, branch: branchOf(worktrees, a.key, open.find((t) => t.agent === a.key)?.path) });
  const unpinned = open.map((t) => t.agent).filter((n) => !pins.includes(n));
  return { pinned: pinnedOf(agents, pins).map(item), open: pinnedOf(agents, unpinned).map(item) };
}

/** The pins with `name` added at the end, or taken out. */
export const togglePin = (pins: readonly string[], name: string): string[] =>
  pins.includes(name) ? pins.filter((n) => n !== name) : [...pins, name];

/** The pins with `name` moved to `index`. */
export function movePin(pins: readonly string[], name: string, index: number): string[] {
  const rest = pins.filter((n) => n !== name);
  if (rest.length === pins.length) return [...pins];
  rest.splice(Math.max(0, Math.min(index, rest.length)), 0, name);
  return rest;
}

/** The open tabs after opening `tab`: an open agent keeps its place and takes the new path; past `max` unpinned, the oldest unpinned goes. */
export function withOpen(open: readonly OpenTab[], tab: OpenTab, pins: readonly string[] = [], max = OPEN_MAX): OpenTab[] {
  if (open.some((t) => t.agent === tab.agent)) return open.map((t) => (t.agent === tab.agent ? tab : t));
  const next = [...open, tab];
  const unpinned = next.filter((t) => !pins.includes(t.agent));
  const drop = new Set(unpinned.slice(0, Math.max(0, unpinned.length - max)));
  return next.filter((t) => !drop.has(t));
}
