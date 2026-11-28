// The people of an office on the move (moves.ts plans each walk): whoever's seat changes while the office is shown walks there, frame by
// frame, and sits once it arrives; its seated figure is not drawn meanwhile. The office first shown, or laid out anew at another width,
// puts everyone where they are with no walk. Lives outside React, as the founder's walk does: only what draws a mover follows it.
import { useLayoutEffect, useState, useSyncExternalStore } from 'react';
import type { Person } from '#src/town/model.ts';
import { type Facing } from '#src/town/people.ts';
import { figuresOf } from '#src/town/sheet.ts';
import { changesOf, routeOf, standingAt, type Door, type Seat } from '#src/town/moves.ts';
import { LIT } from '#src/town/room.ts';
import type { Floor, Spot } from '#src/town/walk.ts';

/** One walking: where it is, the turns ahead, which way it faces and how far it has walked; `leaving` while it goes out of the door. */
export type Mover = {
  at: Spot;
  path: Spot[];
  facing: Facing;
  walked: number;
  row: number;
  name: string;
  leaving: boolean;
};
/** What the crowd needs of an office's drawing: its seats, door and floor, its people, and `layout`, which differs when it is laid out at
 *  another width. */
export type Stage = {
  seats: ReadonlyMap<string, Seat>;
  door: Door;
  floor: Floor;
  people: Person[];
  layout: number;
};
export type Movers = ReadonlyMap<string, Mover>;
export type Crowd = {
  get: () => Movers;
  subscribe: (l: () => void) => () => void;
  place: (s: Stage | null) => void;
  stage: () => Stage | null;
};

/** Metres a second; a little slower than the founder. */
const SPEED = 1.1;

const facingOf = ([dx, dz]: Spot, was: Facing): Facing =>
  Math.abs(dx) > Math.abs(dz) ? (dx < 0 ? 'left' : 'right') : dz < 0 ? 'up' : dz > 0 ? 'down' : was;

/** A mover `dt` seconds on: along its turns, dropping each it reaches. */
export function stepped(m: Mover, dt: number): Mover {
  let [at, path, facing, left] = [m.at, m.path, m.facing, SPEED * dt];
  while (left > 0 && path.length) {
    const [tx, tz] = path[0];
    const [dx, dz] = [tx - at[0], tz - at[1]];
    const far = Math.hypot(dx, dz);
    if (far > 0) facing = facingOf([dx, dz], facing);
    if (far <= left) {
      [at, path, left] = [path[0], path.slice(1), left - far];
      continue;
    }
    at = [at[0] + (dx / far) * left, at[1] + (dz / far) * left];
    left = 0;
  }
  return { ...m, at, path, facing, walked: m.walked + SPEED * dt - left };
}

export function crowdOf(): Crowd {
  const all = new Set<() => void>();
  let movers: Movers = new Map();
  let stage: Stage | null = null;
  let [frame, last] = [0, 0];
  const emit = () => all.forEach((l) => l());
  const tick = (now: number) => {
    const dt = Math.min(0.1, Math.max(0, (now - last) / 1000));
    last = now;
    const next = new Map<string, Mover>();
    for (const [key, m] of movers) {
      const s = stepped(m, dt);
      if (s.path.length) next.set(key, s);
    }
    movers = next;
    emit();
    frame = movers.size ? requestAnimationFrame(tick) : 0;
  };
  const place = (s: Stage | null) => {
    const was = stage;
    stage = s;
    if (!s || !was || was.layout !== s.layout) {
      cancelAnimationFrame(frame);
      frame = 0;
      movers = new Map();
      emit();
      return;
    }
    // Whoever's seat changed walks there; whoever was walking walks on over the new floor, to where its seat is now.
    const changed = new Set(changesOf(was.seats, s.seats).map((c) => c.key));
    // With every seat where it was the floor is too (a screen lit or dark, a paper more): everyone walks on as they were.
    if (!changed.size) return;
    const moving = new Set([...changed, ...movers.keys()]);
    const next = new Map<string, Mover>();
    // One gone from the team walks out as it looked.
    const people = new Map([...was.people, ...s.people].map((p) => [p.key, p]));
    const [rows, rowsWere] = [figuresOf(s.people), figuresOf(was.people)];
    for (const key of moving) {
      const now = movers.get(key);
      const to = s.seats.get(key) ?? null;
      const route = routeOf(s.floor, s.door, now ? standingAt(s.floor, now.at) : (was.seats.get(key) ?? null), to);
      const row = rows.get(key) ?? rowsWere.get(key) ?? 0;
      const m = { at: route[0], path: route.slice(1), row, name: people.get(key)?.name ?? key, leaving: !to };
      next.set(key, { facing: 'down', walked: 0, ...now, ...m });
    }
    movers = next;
    emit();
    go();
  };
  const go = () => {
    if (frame) return;
    last = performance.now();
    frame = requestAnimationFrame(tick);
  };
  return { get: () => movers, subscribe: (l) => (all.add(l), () => void all.delete(l)), place, stage: () => stage };
}

/** An office's crowd, kept while the office is shown. */
export const useCrowd = (): Crowd => useState(crowdOf)[0];

/** `crowd` placed on the office drawn as `stage` before the screen is painted, so a seated figure never shows at its new seat. */
export function usePlaced(crowd: Crowd, stage: Stage | null) {
  useLayoutEffect(() => crowd.place(stage), [crowd, stage]);
  useLayoutEffect(() => () => crowd.place(null), [crowd]);
}

export const useMovers = (c: Crowd) => useSyncExternalStore(c.subscribe, c.get);

/** Who is walking to its desk on `stage` with a screen to light there, comma-joined by key: it lights once it sits. */
export const toDesks = (movers: Movers, stage: Stage | null): string =>
  [...movers]
    .filter(([key, m]) => !m.leaving && stage?.seats.get(key)?.id.startsWith('desk:'))
    .filter(([key]) => LIT.some((a) => stage?.people.find((p) => p.key === key)?.activity === a))
    .map(([key]) => key)
    .sort()
    .join(',');

/** The keys walking to their desks in `crowd`, as toDesks says, kept as it changes; read before the drawing it darkens, so it lags the
 *  crowd by the one render the drawing takes. */
export function useToDesks(crowd: Crowd): string {
  const [keys, setKeys] = useState('');
  useLayoutEffect(() => {
    const read = () => setKeys(toDesks(crowd.get(), crowd.stage()));
    read();
    return crowd.subscribe(read);
  }, [crowd]);
  return keys;
}
