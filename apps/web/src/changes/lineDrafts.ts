// The line comments a person has written and not sent yet, per place they go to: a task, or an agent's worktree, on the cluster the
// view reads. Kept across a reload (persist/kv.ts) and read by every view of the place at once (a task's Code and its Redo button), so a
// comment added in one shows in the other. Nothing here is sent: the drafts go out once, together, and a send drops the ones it took.
import { draftsKey, movedDrafts, type DiffFile, type LineComment } from '@handline/app-core';
import { useEffect, useEffectEvent, useSyncExternalStore } from 'react';
import { useCluster } from '#src/connection/clusters.tsx';
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
  /** The name they are kept under: the place and its cluster. What else is kept per place (a box still open) goes by it too. */
  key: string;
  list: LineComment[];
  /** The drafts as they are this moment, for a send: it sends these and later drops these, whatever was added meanwhile. */
  now: () => LineComment[];
  add: (c: Omit<LineComment, 'id'>) => void;
  edit: (id: string, text: string) => void;
  /** Drops these: one the person removed, or the ones a send took. */
  drop: (ids: string[]) => void;
  /** Marks these as moved: their line is not in the diff as it was (LineComment.moved). */
  moved: (ids: string[]) => void;
}

const newId = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;

/** The drafts of `place` (`task:<id>` or `agent:<name>`) on the cluster the view reads, kept per browser as a chat's draft is. */
export function useLineDrafts(place: string): LineDrafts {
  const name = draftsKey(useCluster(), place);
  const list = useSyncExternalStore(subscribe, () => get(name));
  const each = (f: (c: LineComment) => LineComment) => set(name, get(name).map(f));
  return {
    key: name,
    list,
    now: () => get(name),
    add: (c) => set(name, [...get(name), { ...c, id: newId() }]),
    edit: (id, text) => each((c) => (c.id === id ? { ...c, text } : c)),
    drop: (ids) =>
      set(
        name,
        get(name).filter((c) => !ids.includes(c.id)),
      ),
    moved: (ids) => each((c) => (ids.includes(c.id) ? { ...c, moved: true } : c)),
  };
}

/** Marks the drafts written on the diff shown (`view`) whose line that diff no longer has as written, once the diff is read (`files`;
 *  null while it loads). The mark is kept with the draft, so a send from a screen that shows no diff (Redo) knows of it too. */
export function useMovedMarked(drafts: LineDrafts, view: string, files: DiffFile[] | null) {
  const ids = files ? movedDrafts(drafts.list, files, view).join(' ') : '';
  const mark = useEffectEvent((moved: string[]) => drafts.moved(moved));
  useEffect(() => {
    if (ids) mark(ids.split(' '));
  }, [ids]);
}

export const taskPlace = (task: number) => `task:${task}`;
export const agentPlace = (agent: string) => `agent:${agent}`;
