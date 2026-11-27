// The bubble that pops over a person when its work changes while its office is on screen (Marks.tsx draws it): the new work in a few
// words, for a few seconds. Nothing pops when the office is first shown, only on a change seen since; and one whose work changes all
// the time (each tool call of an agent with no task is new work) pops again only after a quiet while. What pops and when is plain sums
// (`popped`, `showing`); the store around them lives outside React, as the crowd does, and only what draws the bubbles follows it.
import { useEffect, useState, useSyncExternalStore } from 'react';
import type { Person } from '#src/town/model.ts';
import { cutTo } from '#src/town/marks.ts';

/** How long a bubble shows. */
export const POP_MS = 4000;
/** From one bubble of a person to its next, at least. */
export const QUIET_MS = 12_000;
/** A bubble's text at its longest, in letters. */
export const POP_LETTERS = 40;

/** A bubble over `key` saying `text`, begun at `at` (ms). */
export type Pop = { key: string; text: string; at: number };
/** What an office last saw: its people (null before the first look) and the bubbles begun not long ago, shown or only kept so their
 *  person stays quiet. */
export type Popped = { people: readonly Person[] | null; pops: readonly Pop[] };

/** Whose work is new between two looks at the same office, with the new work: there both times, doing something now, and not what it
 *  did before. One who came in since, or stopped, is not news. */
export function changedWork(was: readonly Person[], now: readonly Person[]): { key: string; text: string }[] {
  const before = new Map(was.map((p) => [p.key, p.doing?.trim() || null]));
  return now.flatMap((p) => {
    const text = p.doing?.trim();
    return text && before.has(p.key) && before.get(p.key) !== text ? [{ key: p.key, text }] : [];
  });
}

/** The office after a look at `people` at `at`: a bubble begins for each whose work changed, unless its last one began under QUIET_MS
 *  ago; bubbles older than that are forgotten. The first look pops nothing. */
export function popped(was: Popped, people: readonly Person[], at: number): Popped {
  const kept = was.pops.filter((p) => at - p.at < QUIET_MS);
  const fresh = (was.people ? changedWork(was.people, people) : [])
    .filter((c) => !kept.some((p) => p.key === c.key))
    .map((c) => ({ key: c.key, text: cutTo(c.text, POP_LETTERS), at }));
  return { people, pops: fresh.length || kept.length !== was.pops.length ? [...kept, ...fresh] : was.pops };
}

/** The bubbles on screen at `at`: those begun under POP_MS ago. */
export const showing = (pops: readonly Pop[], at: number): Pop[] => pops.filter((p) => at - p.at < POP_MS);

export type Pops = {
  get: () => readonly Pop[];
  subscribe: (l: () => void) => () => void;
  /** Looks at the office's people now; null when the office goes away. */
  see: (people: readonly Person[] | null) => void;
};

/** An office's bubbles, kept as time passes: each goes when its time is up. */
export function popsOf(clock: () => number = Date.now): Pops {
  const all = new Set<() => void>();
  let state: Popped = { people: null, pops: [] };
  let shown: readonly Pop[] = [];
  let timer: ReturnType<typeof setTimeout> | undefined;
  const look = () => {
    clearTimeout(timer);
    const at = clock();
    const now = showing(state.pops, at);
    if (now.length !== shown.length || now.some((p, i) => p !== shown[i])) {
      shown = now;
      all.forEach((l) => l());
    }
    if (now.length) timer = setTimeout(look, Math.min(...now.map((p) => p.at + POP_MS)) - at);
  };
  const see = (people: readonly Person[] | null) => {
    state = people ? popped(state, people, clock()) : { people: null, pops: [] };
    look();
  };
  return { get: () => shown, subscribe: (l) => (all.add(l), () => void all.delete(l)), see };
}

/** The bubbles to draw over `people` now, as their work changes while this is on screen. */
export function usePops(people: readonly Person[]): readonly Pop[] {
  const [pops] = useState(() => popsOf());
  useEffect(() => pops.see(people), [pops, people]);
  useEffect(() => () => pops.see(null), [pops]);
  return useSyncExternalStore(pops.subscribe, pops.get);
}
