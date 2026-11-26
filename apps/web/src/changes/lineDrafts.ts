// The line comments a person has written and not sent yet, per place they go to: a task, or an agent's worktree. Kept across a reload
// (persist/kv.ts) and read by every view of the place at once (a task's Code and its Redo button), so a comment added in one shows in the
// other. Nothing here is sent: the drafts go out once, together, and are cleared only when that went through.
import type { LineComment } from '@handline/app-core';
import { useSyncExternalStore } from 'react';
import { read, write } from '#src/persist/kv.ts';

const NONE: LineComment[] = [];
const held = new Map<string, LineComment[]>();
const listeners = new Set<() => void>();

const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => void listeners.delete(l);
};

function get(name: string): LineComment[] {
  if (!held.has(name)) {
    const kept = read<unknown>(name, null);
    held.set(name, Array.isArray(kept) && kept.length ? (kept as LineComment[]) : NONE);
  }
  return held.get(name)!;
}

function set(name: string, next: LineComment[]) {
  held.set(name, next.length ? next : NONE);
  write(name, next.length ? next : undefined);
  for (const l of listeners) l();
}

/** What a file's view needs to draw and change the drafts of the place it shows. */
export interface LineDrafts {
  list: LineComment[];
  add: (c: Omit<LineComment, 'id'>) => void;
  edit: (id: string, text: string) => void;
  remove: (id: string) => void;
  /** Drops them all: after they were sent. */
  clear: () => void;
}

const newId = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;

/** The drafts of `place` (`task:<id>` or `agent:<name>`), kept per browser as a chat's draft is. */
export function useLineDrafts(place: string): LineDrafts {
  const name = `linecomments/${place}`;
  const list = useSyncExternalStore(subscribe, () => get(name));
  return {
    list,
    add: (c) => set(name, [...get(name), { ...c, id: newId() }]),
    edit: (id, text) =>
      set(
        name,
        get(name).map((c) => (c.id === id ? { ...c, text } : c)),
      ),
    remove: (id) =>
      set(
        name,
        get(name).filter((c) => c.id !== id),
      ),
    clear: () => set(name, []),
  };
}

export const taskPlace = (task: number) => `task:${task}`;
export const agentPlace = (agent: string) => `agent:${agent}`;
