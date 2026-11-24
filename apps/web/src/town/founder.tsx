// The founder, from public/town/founder.png: standing and walking, facing down, up and left (right is left mirrored). In the city and in
// an office it walks where a click sends it, or where the arrow keys (or WASD) push it; in an office the room's nearer pixels are laid
// over it, as over the seated people. Its place lives outside React: only what draws it, or picks something by it, follows it.
import { useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { FLAT, frontPixels, type Box } from '#src/town/people.ts';
import type { Picture } from '#src/town/shade.ts';
import { NAME_HALO, NAME_INK } from '#src/town/palette.ts';
import { freeAt, nearestFree, pathTo, step, type Floor, type Spot } from '#src/town/walk.ts';

export const FOUNDER_SHEET = `${import.meta.env.BASE_URL}town/founder.png`;
const CELL = [136, 226] as const;
const SIZE = [816, 226] as const;

type Facing = 'down' | 'up' | 'left' | 'right';
const COL: Record<Facing, number> = { down: 0, up: 2, left: 4, right: 4 };
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
  kinds: string[];
  toPixel: (x: number, y: number, z: number) => [number, number];
  /** The floor spot drawn at picture pixel (px, py). */
  toFloor: (px: number, py: number) => Spot;
  depthOf: (x: number, y: number, z: number) => number;
  /** How many picture pixels a sheet pixel of the founder takes here. */
  sprite: number;
};
export type Walker = { at: Spot; facing: Facing; walked: number; moving: boolean };
/** The founder's walk in one place: `go` walks to a spot and then does `then`; `get` and `subscribe` follow where it is. */
export type Founder = { go: (to: Spot, then?: () => void) => void; get: () => Walker | null; subscribe: (l: () => void) => () => void };

const facingOf = ([dx, dz]: Spot, was: Facing): Facing =>
  Math.abs(dx) > Math.abs(dz) ? (dx < 0 ? 'left' : 'right') : dz < 0 ? 'up' : dz > 0 ? 'down' : was;
/** Keys typed into a field, or anywhere in a floating window, are the window's, not the founder's. */
const notMine = (t: EventTarget | null) =>
  t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName) || !!t.closest('[data-no-walk]'));
const sameFloor = (a: Floor, b: Floor) =>
  a === b || (a.cols === b.cols && a.rows === b.rows && a.cell === b.cell && a.free.every((v, i) => v === b.free[i]));
const standing = (at: Spot): Walker => ({ at, facing: 'down', walked: 0, moving: false });

/** Where the founder last stood in each place, so a view drawn again keeps it there; another entry (another door) starts it there. */
const kept = new Map<string, { entry: string; me: Walker }>();

/** The founder's state and its frame loop, which runs only while the founder moves. */
function walkerOf() {
  const all = new Set<() => void>();
  const bump: { current?: (at: Spot, dir: Spot) => void } = {};
  const s = {
    me: null as Walker | null,
    ground: null as Walk | null,
    plan: null as { path: Spot[]; then?: () => void } | null,
    held: new Set<string>(),
    frame: 0,
    last: 0,
  };
  const emit = () => all.forEach((l) => l());
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
    let next = m;
    if (dir[0] || dir[1]) {
      s.plan = null;
      const at = step(g.floor, m.at, dir, g.speed * dt);
      if (at === m.at) bump.current?.(m.at, dir);
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
      if (m.moving) next = { ...m, moving: false };
      then?.();
    }
    if (next !== m) {
      s.me = next;
      emit();
    }
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
      const path = pathTo(s.ground.floor, s.me.at, to);
      s.plan = path ? { path, then } : null;
      wake();
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
    // The room laid out again (another bench width) may have put furniture where the founder stood.
    if (!freeAt(ground.floor, me.at)) me = { ...me, at: nearestFree(ground.floor, me.at) ?? ground.entry };
    s.me = me;
    kept.set(where, { entry, me });
    emit();
    return () => {
      if (s.me) kept.set(where, { entry, me: s.me });
    };
  };
  /** Walks the founder by the keys while on. A key held from before (it walked the founder into this place) only repeats here: it does
   *  not walk the founder on. */
  const listen = () => {
    const down = (e: KeyboardEvent) => {
      if (!KEYS[e.code] || e.repeat || !s.ground || notMine(e.target) || e.metaKey || e.ctrlKey || e.altKey) return;
      e.preventDefault();
      s.held.add(e.code);
      wake();
    };
    const up = (e: KeyboardEvent) => s.held.delete(e.code);
    const blur = () => s.held.clear();
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
  const onBump = (f?: (at: Spot, dir: Spot) => void) => {
    bump.current = f;
  };
  return { founder, place, listen, onBump };
}

/** The founder in `where` (the city, or a house's key) on `ground`; none while `ground` is null. `onBump` hears where held keys push into
 *  something. */
export function useWalker(ground: Walk | null, where: string, onBump?: (at: Spot, dir: Spot) => void): Founder {
  const [w] = useState(walkerOf);
  useEffect(() => w.onBump(onBump));
  useEffect(() => w.place(ground, where), [w, ground, where]);
  useEffect(() => w.listen(), [w]);
  return w.founder;
}

/** What `pick` makes of the founder's place, drawn again only when that changes: keep it a string, a number or null. */
export function useFounder<T extends string | number | null>(f: Founder, pick: (me: Walker | null) => T): T {
  return useSyncExternalStore(f.subscribe, () => pick(f.get()));
}

const useMe = (f: Founder) => useSyncExternalStore(f.subscribe, f.get);

/** The founder's box with its feet at (fx, fy), at `scale` picture pixels per sheet pixel. */
const boxAt = (fx: number, fy: number, scale: number): Box => {
  const [w, h] = [CELL[0] * scale, CELL[1] * scale];
  return { x: fx - w / 2, y: fy + scale - h, w, h };
};

/** The founder's figure in `box`, in the frame for its facing and stride. */
function Sprite({ me, box, stride }: { me: Walker; box: Box; stride: number }) {
  const col = COL[me.facing] + (me.moving && Math.floor(me.walked / stride) % 2 === 1 ? 1 : 0);
  return (
    <g transform={me.facing === 'right' ? `translate(${2 * box.x + box.w} 0) scale(-1 1)` : undefined} className="pointer-events-none">
      <svg x={box.x} y={box.y} width={box.w} height={box.h} viewBox={`${col * CELL[0]} 0 ${CELL[0]} ${CELL[1]}`}>
        <image href={FOUNDER_SHEET} width={SIZE[0]} height={SIZE[1]} />
      </svg>
    </g>
  );
}

/** The founder in an office or the city, with whatever stands nearer laid over it, drawn into one canvas as it walks. */
/** `label` is drawn under the feet, as each agent's name is. */
export function FounderDrawing({ founder, ground, label }: { founder: Founder; ground: Ground; label?: string }) {
  const me = useMe(founder);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [x, z] = me?.at ?? ground.entry;
  const [fx, fy] = ground.toPixel(x, 0, z);
  const box = boxAt(fx, fy, ground.sprite);
  // The overlay is whole picture pixels around the figure.
  const [bx, by, bw, bh] = [Math.floor(box.x), Math.floor(box.y), Math.ceil(box.w) + 1, Math.ceil(box.h) + 1];
  const depth = ground.depthOf(x, 0.45, z);
  useEffect(() => {
    const c = canvas.current;
    if (!c) return;
    const { pic, kinds } = ground;
    [c.width, c.height] = [bw, bh];
    const front = frontPixels(pic, { x: bx, y: by, w: bw, h: bh }, (i) => !FLAT.has(kinds[pic.id[i]] ?? '') && pic.depth[i] < depth);
    if (front) c.getContext('2d')?.putImageData(front.img, front.x - bx, front.y - by);
  }, [ground, bx, by, bw, bh, depth]);
  if (!me) return null;
  return (
    <g className="pointer-events-none">
      <Sprite me={me} box={box} stride={ground.stride} />
      <foreignObject x={bx} y={by} width={bw} height={bh}>
        <canvas ref={canvas} style={{ display: 'block', width: '100%', height: '100%', imageRendering: 'pixelated' }} />
      </foreignObject>
      {label && (
        <text
          x={fx}
          y={fy + 9}
          textAnchor="middle"
          fontSize={8}
          fontWeight={700}
          fill={NAME_INK}
          stroke={NAME_HALO}
          strokeWidth={2.5}
          paintOrder="stroke"
        >
          {label}
        </text>
      )}
    </g>
  );
}
