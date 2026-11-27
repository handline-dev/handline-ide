// The founder, in the look picked for it (look.ts): standing, left foot and right foot, facing down, up and left (right is left mirrored). In the city and in
// an office it walks where a click sends it, or where the arrow keys (or WASD) push it, running while Shift is held; in an office the
// room's nearer pixels are laid over it, as over the seated people. Its place lives outside React: only what draws it, or picks something
// by it, follows it.
import type { Screen } from '#src/town/monitors.tsx';
import { useEffect, useState, useSyncExternalStore } from 'react';
import type { Box } from '#src/town/people.ts';
import type { Picture } from '#src/town/shade.ts';
import { boxOn, WalkingFigure } from '#src/town/walking.tsx';
import { LOOKS, lookNow, useLook } from '#src/town/look.ts';
import { freeAt, nearestFree, pathTo, reach, step, type Floor, type Spot } from '#src/town/walk.ts';

export type Facing = 'down' | 'up' | 'left' | 'right';
const COL: Record<Facing, number> = { down: 0, up: 3, left: 6, right: 6 };
/** The walk: standing, left foot, standing, right foot; each facing has those three frames in a row. */
const WALK = [0, 1, 0, 2] as const;
/** By physical key, so Shift or Caps Lock never strands one held. */
const KEYS: Record<string, Spot> = {
  ArrowUp: [0, -1],
  ArrowDown: [0, 1],
  ArrowLeft: [-1, 0],
  ArrowRight: [1, 0],
  KeyW: [0, -1],
  KeyS: [0, 1],
  KeyA: [-1, 0],
  KeyD: [1, 0],
};

/** Where the founder walks: the grid, where it comes in, how fast it walks and how far between its two feet frames, in the ground's own
 *  unit a second. */
export type Walk = { floor: Floor; entry: Spot; speed: number; stride: number };
/** An office's ground: its walk, its pixels and objects, and where a floor spot or a point lands in the picture. */
export type Ground = Walk & {
  pic: Picture;
  /** By the picture's object ids: what never covers the founder, as the floor and a door it walks through; and each one's tag. */
  under: boolean[];
  tags: (string | undefined)[];
  toPixel: (x: number, y: number, z: number) => [number, number];
  /** The floor spot drawn at picture pixel (px, py). */
  toFloor: (px: number, py: number) => Spot;
  depthOf: (x: number, y: number, z: number) => number;
  /** How many picture pixels a sheet pixel of the founder takes here. */
  sprite: number;
  /** The screens that move (an office's working ones), laid over a walker they stand in front of as they move. */
  screens?: Screen[];
};
/** `seated`: in a desk chair, facing the desk. */
export type Walker = { at: Spot; facing: Facing; walked: number; moving: boolean; seated?: boolean };
/** The founder's walk in one place: `go` walks to a spot and then does `then`; `put` stands it at a spot at once, facing away, as before
 *  a door; `turn` turns it where it stands; `sit` walks it to `side`, beside a desk chair, and sideways into the chair at `seat`, from
 *  where it gets up to `side` again when it walks on; `get` and `subscribe` follow where it is. */
export type Founder = {
  go: (to: Spot, then?: () => void) => void;
  put: (to: Spot) => void;
  turn: (facing: Facing) => void;
  sit: (seat: Spot, side: Spot) => void;
  get: () => Walker | null;
  subscribe: (l: () => void) => () => void;
};

const facingOf = ([dx, dz]: Spot, was: Facing): Facing =>
  Math.abs(dx) > Math.abs(dz) ? (dx < 0 ? 'left' : 'right') : dz < 0 ? 'up' : dz > 0 ? 'down' : was;
/** Keys typed into a field, anywhere in a floating window or in a menu, are theirs, not the founder's; and while a form or a panel is
 *  open over the town (a modal, a popover), every key is that one's. */
export const notMine = (t: EventTarget | null) =>
  !!document.querySelector('[role="dialog"], [role="alertdialog"]') ||
  (t instanceof HTMLElement &&
    (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName) || !!t.closest('[data-no-walk], [role="menu"]')));
const sameFloor = (a: Floor, b: Floor) =>
  a === b || (a.cols === b.cols && a.rows === b.rows && a.cell === b.cell && a.free.every((v, i) => v === b.free[i]));
const standing = (at: Spot): Walker => ({ at, facing: 'down', walked: 0, moving: false });

/** Where the founder last stood in each place, so a view drawn again keeps it there; another entry (another door) starts it there. */
const kept = new Map<string, { entry: string; me: Walker }>();

/** The founder's state and its frame loop, which runs only while the founder moves. */
function walkerOf() {
  const all = new Set<() => void>();
  const bump: { current?: Bump } = {};
  const s = {
    me: null as Walker | null,
    ground: null as Walk | null,
    plan: null as { path: Spot[]; then?: () => void } | null,
    /** Where the founder gets up to from the chair it sits in. */
    side: null as Spot | null,
    held: new Set<string>(),
    running: false,
    frame: 0,
    last: 0,
  };
  const emit = () => all.forEach((l) => l());
  /** Where one off free floor (in a chair, or half out of it) gets out to: beside the chair it sat down from, when that is the one it is
   *  in, else the nearest free spot. */
  const outOf = (floor: Floor, at: Spot): Spot =>
    s.side && Math.hypot(s.side[0] - at[0], s.side[1] - at[1]) < 0.6 ? s.side : (nearestFree(floor, at) ?? at);
  const stop = () => {
    cancelAnimationFrame(s.frame);
    s.frame = 0;
  };
  const tick = (now: number) => {
    s.frame = 0;
    const g = s.ground;
    const m = s.me;
    if (!g || !m) return;
    const dt = Math.min(0.1, Math.max(0, (now - s.last) / 1000));
    s.last = now;
    const dirs = [...s.held].map((k) => KEYS[k]);
    const dir: Spot = [dirs.reduce((n, d) => n + d[0], 0), dirs.reduce((n, d) => n + d[1], 0)];
    // Pushed by a key in or by a chair, off free floor, the founder first gets out beside it.
    const off = !freeAt(g.floor, m.at);
    if (off && (dir[0] || dir[1]) && !s.plan) s.plan = { path: [outOf(g.floor, m.at)] };
    let next: Walker;
    if ((dir[0] || dir[1]) && !off) {
      s.plan = null;
      const at = step(g.floor, m.at, dir, reach(g.speed, dt, s.running));
      if (at === m.at) bump.current?.(m.at, dir, founder);
      next = { at, facing: facingOf(dir, m.facing), walked: m.walked + Math.hypot(at[0] - m.at[0], at[1] - m.at[1]), moving: true };
    } else if (s.plan?.path.length) {
      const [tx, tz] = s.plan.path[0];
      const [dx, dz] = [tx - m.at[0], tz - m.at[1]];
      const left = Math.hypot(dx, dz);
      const move = g.speed * dt;
      if (left <= move) s.plan.path.shift();
      const at: Spot = left <= move ? [tx, tz] : [m.at[0] + (dx / left) * move, m.at[1] + (dz / left) * move];
      next = { at, facing: facingOf([dx, dz], m.facing), walked: m.walked + Math.min(move, left), moving: true };
    } else {
      const then = s.plan?.then;
      s.plan = null;
      // A key still held walks on next frame: one that got the founder up out of a chair. What comes after the walk sees it stopped,
      // and what it changes is not undone here.
      if (m.moving) {
        s.me = { ...m, moving: !!(dir[0] || dir[1]) };
        emit();
      }
      then?.();
      if (s.me?.moving && !s.frame) s.frame = requestAnimationFrame(tick);
      return;
    }
    s.me = next;
    emit();
    if (next.moving) s.frame = requestAnimationFrame(tick);
  };
  const wake = () => {
    if (s.frame || !s.ground) return;
    s.last = performance.now();
    s.frame = requestAnimationFrame(tick);
  };
  const founder: Founder = {
    go: (to, then) => {
      if (!s.ground || !s.me) return;
      // From a chair, or on the way into or out of one, out to its side first: the chair is not free floor.
      const from = freeAt(s.ground.floor, s.me.at) ? s.me.at : outOf(s.ground.floor, s.me.at);
      const path = pathTo(s.ground.floor, from, to);
      s.plan = path ? { path: from === s.me.at ? path : [from, ...path], then } : null;
      wake();
    },
    put: (to) => {
      if (!s.ground || !s.me) return;
      s.plan = null;
      s.me = { ...s.me, at: nearestFree(s.ground.floor, to) ?? s.me.at, facing: 'up', moving: false, seated: false };
      emit();
    },
    turn: (facing) => {
      if (!s.me || s.me.moving || s.me.seated) return;
      s.me = { ...s.me, facing };
      emit();
    },
    sit: (seat, side) => {
      if (s.me?.seated && s.me.at[0] === seat[0] && s.me.at[1] === seat[1]) return;
      founder.go(side, () => {
        // From here on, a walk out of the chair goes back out this side.
        s.side = side;
        s.plan = {
          path: [seat],
          then: () => {
            if (!s.me) return;
            s.me = { ...s.me, facing: 'up', seated: true };
            emit();
          },
        };
        wake();
      });
    },
    get: () => s.me,
    subscribe: (l) => (all.add(l), () => void all.delete(l)),
  };
  /** Puts the founder on `ground` in `where`: it stays where it stood when the entry is the same, else starts at the entry; a click's walk
   *  goes on only over the same floor. Returns what keeps its place when it leaves. */
  const place = (ground: Walk | null, where: string) => {
    const was = s.ground;
    s.ground = ground;
    if (!ground) {
      stop();
      s.held.clear();
      s.plan = null;
      return undefined;
    }
    if (!was || !sameFloor(was.floor, ground.floor)) s.plan = null;
    const entry = ground.entry.join(',');
    const k = kept.get(where);
    let me = k?.entry === entry ? (was && s.me ? s.me : k.me) : standing(ground.entry);
    // The room laid out again (another team size) may have put furniture where the founder stood; one seated in the same room stays.
    const same = was && sameFloor(was.floor, ground.floor);
    if (me.seated && !same) me = { ...me, seated: false };
    if (!freeAt(ground.floor, me.at) && !me.seated) me = { ...me, at: nearestFree(ground.floor, me.at) ?? ground.entry };
    if (!same) s.side = null;
    s.me = me;
    kept.set(where, { entry, me });
    emit();
    return () => {
      if (s.me) kept.set(where, { entry, me: s.me });
    };
  };
  /** Walks the founder by the keys while on, at a run while Shift is down. A key held from before (it walked the founder into this place)
   *  only repeats here: it does not walk the founder on. */
  const listen = () => {
    const down = (e: KeyboardEvent) => {
      s.running = e.shiftKey;
      if (!KEYS[e.code] || !s.ground || notMine(e.target) || e.metaKey || e.ctrlKey || e.altKey) return;
      // A repeat walks nobody on, but it must not scroll a big office either.
      e.preventDefault();
      if (e.repeat) return;
      s.held.add(e.code);
      wake();
    };
    const up = (e: KeyboardEvent) => {
      s.running = e.shiftKey;
      s.held.delete(e.code);
    };
    const blur = () => {
      s.running = false;
      s.held.clear();
    };
    window.addEventListener('keydown', down);
    window.addEventListener('keyup', up);
    window.addEventListener('blur', blur);
    return () => {
      stop();
      s.held.clear();
      window.removeEventListener('keydown', down);
      window.removeEventListener('keyup', up);
      window.removeEventListener('blur', blur);
    };
  };
  const onBump = (f?: Bump) => {
    bump.current = f;
  };
  return { founder, place, listen, onBump };
}

/** Where held keys push the founder into something, and which way. */
type Bump = (at: Spot, dir: Spot, founder: Founder) => void;

/** The founder in `where` (the city, or a house's key) on `ground`; none while `ground` is null. `onBump` hears where held keys push into
 *  something. */
export function useWalker(ground: Walk | null, where: string, onBump?: Bump): Founder {
  const [w] = useState(walkerOf);
  useEffect(() => w.onBump(onBump));
  useEffect(() => w.place(ground, where), [w, ground, where]);
  useEffect(() => w.listen(), [w]);
  return w.founder;
}

/** What `pick` makes of the founder's place, drawn again only when that changes: keep it a string, a number, a boolean or null. */
export function useFounder<T extends string | number | boolean | null>(f: Founder, pick: (me: Walker | null) => T): T {
  return useSyncExternalStore(f.subscribe, () => pick(f.get()));
}

const useMe = (f: Founder) => useSyncExternalStore(f.subscribe, f.get);

/** The founder's box on the picture standing at (x, z) on `ground`. */
export const founderBox = (ground: Ground, at: Spot): Box => boxOn(ground, LOOKS[lookNow()].sheet, at);

/** The founder in an office or the city, in the frame for its facing and stride, with whatever stands nearer laid over it. `label` is
 *  drawn under the feet, as each agent's name is. */
export function FounderDrawing({ founder, ground, label }: { founder: Founder; ground: Ground; label?: string }) {
  const me = useMe(founder);
  const look = useLook();
  if (!me) return null;
  const col = COL[me.facing] + (me.moving ? WALK[Math.floor(me.walked / ground.stride) % 4] : 0);
  return (
    <WalkingFigure
      sit={me.seated ? look.sit : undefined}
      ground={ground}
      sheet={look.sheet}
      at={me.at}
      col={col}
      row={look.row}
      flip={me.facing === 'right'}
      label={label ? { text: label, size: 8, halo: 2.5 } : undefined}
    />
  );
}
