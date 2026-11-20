// Which agents the agent list shows, from the person's prefs. Pure.
import type { NavPrefs, Prefs } from '@handline/client';

export const DEFAULT_NAV: NavPrefs = { pins: [], unread_only: false, hidden: [], layout: 'tree', order: [], folded: [] };

export const navOf = (p: Prefs | undefined): NavPrefs => ({ ...DEFAULT_NAV, ...p?.nav });

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
