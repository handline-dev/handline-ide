// The founder, from public/town/founder.png: standing and walking, facing down, up and left (right is left mirrored). In the city and in
// an office it walks where a click sends it, or where the arrow keys (or WASD) push it; in an office the room's nearer pixels are laid
// over it, as over the seated people.
import { useEffect, useMemo, useRef, useState } from 'react';
import { FLAT, frontOf, SCALE, type Box, type Figure } from '#src/town/people.ts';
import type { Picture } from '#src/town/shade.ts';
import { freeAt, nearestFree, pathTo, step, type Floor, type Spot } from '#src/town/walk.ts';

export const FOUNDER_SHEET = `${import.meta.env.BASE_URL}town/founder.png`;
const CELL = [136, 226] as const;
const SIZE = [816, 226] as const;

type Facing = 'down' | 'up' | 'left' | 'right';
const COL: Record<Facing, number> = { down: 0, up: 2, left: 4, right: 4 };
const KEYS: Record<string, Spot> = {
  ArrowUp: [0, -1],
  ArrowDown: [0, 1],
  ArrowLeft: [-1, 0],
  ArrowRight: [1, 0],
  w: [0, -1],
  s: [0, 1],
  a: [-1, 0],
  d: [1, 0],
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
};
export type Walker = { at: Spot; facing: Facing; walked: number; moving: boolean };

const facingOf = ([dx, dz]: Spot, was: Facing): Facing =>
  Math.abs(dx) < 1e-6 && Math.abs(dz) < 1e-6 ? was : Math.abs(dx) > Math.abs(dz) ? (dx < 0 ? 'left' : 'right') : dz < 0 ? 'up' : 'down';
const typing = (t: EventTarget | null) => t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));

/** Where the founder last stood, so a view drawn again keeps it there; going somewhere else starts at that place's entry. */
let kept: { where: string; me: Walker } | null = null;

/** The founder's place and walk in `where` (the city, or a house's key): `go` walks to a spot and then does `then`; held keys walk as they
 *  point, and `onBump` hears where they push into something. */
export function useWalker(ground: Walk | null, where: string, onBump?: (at: Spot, dir: Spot) => void) {
  const [moved, setMe] = useState<Walker | null>(() => (kept?.where === where ? kept.me : null));
  useEffect(() => {
    if (moved) kept = { where, me: moved };
  }, [moved, where]);
  const bump = useRef(onBump);
  useEffect(() => {
    bump.current = onBump;
  });
  // The founder stands inside the door until it first moves.
  const me = moved ?? (ground ? { at: ground.entry, facing: 'down' as const, walked: 0, moving: false } : null);
  const plan = useRef<{ path: Spot[]; then?: () => void } | null>(null);
  const held = useRef(new Set<string>());

  useEffect(() => {
    if (!ground) return;
    let last = performance.now();
    let frame = 0;
    const tick = (now: number) => {
      // A frame's time can be from before the loop started: never a step back.
      const dt = Math.min(0.1, Math.max(0, (now - last) / 1000));
      last = now;
      setMe((was) => {
        let m = was ?? { at: ground.entry, facing: 'down' as const, walked: 0, moving: false };
        // The room laid out again (another bench width) may have put furniture where the founder stood.
        if (!freeAt(ground.floor, m.at)) m = { ...m, at: nearestFree(ground.floor, m.at) ?? ground.entry };
        const keys = [...held.current].map((k) => KEYS[k]).filter(Boolean);
        if (keys.length) {
          plan.current = null;
          const dir: Spot = [keys.reduce((n, k) => n + k[0], 0), keys.reduce((n, k) => n + k[1], 0)];
          const at = step(ground.floor, m.at, dir, ground.speed * dt);
          if (at === m.at) queueMicrotask(() => bump.current?.(m.at, dir));
          return { at, facing: facingOf(dir, m.facing), walked: m.walked + Math.hypot(at[0] - m.at[0], at[1] - m.at[1]), moving: true };
        }
        const p = plan.current;
        if (!p?.path.length) {
          if (p) {
            plan.current = null;
            queueMicrotask(() => p.then?.());
          }
          return m.moving ? { ...m, moving: false } : m;
        }
        const [tx, tz] = p.path[0];
        const [dx, dz] = [tx - m.at[0], tz - m.at[1]];
        const left = Math.hypot(dx, dz);
        const move = ground.speed * dt;
        if (left <= move) p.path.shift();
        const at: Spot = left <= move ? [tx, tz] : [m.at[0] + (dx / left) * move, m.at[1] + (dz / left) * move];
        return { at, facing: facingOf([dx, dz], m.facing), walked: m.walked + Math.min(move, left), moving: true };
      });
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    const down = (e: KeyboardEvent) => {
      if (KEYS[e.key] && !typing(e.target) && !e.metaKey && !e.ctrlKey && !e.altKey) {
        held.current.add(e.key);
        e.preventDefault();
      }
    };
    const up = (e: KeyboardEvent) => held.current.delete(e.key);
    const blur = () => held.current.clear();
    window.addEventListener('keydown', down);
    window.addEventListener('keyup', up);
    window.addEventListener('blur', blur);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener('keydown', down);
      window.removeEventListener('keyup', up);
      window.removeEventListener('blur', blur);
    };
  }, [ground]);

  const go = (to: Spot, then?: () => void) => {
    if (!ground || !me) return;
    const path = pathTo(ground.floor, me.at, to);
    plan.current = path ? { path, then } : null;
  };
  return { me, go };
}

/** The founder's box with its feet at (fx, fy), at `scale` picture pixels per sheet pixel. */
const boxAt = (fx: number, fy: number, scale: number): Box => {
  const [w, h] = [CELL[0] * scale, CELL[1] * scale];
  return { x: fx - w / 2, y: fy + scale - h, w, h };
};

/** The founder's figure in `box`, in the frame for its facing and stride. */
export function FounderSprite({ me, box, stride }: { me: Walker; box: Box; stride: number }) {
  const col = COL[me.facing] + (me.moving && Math.floor(me.walked / stride) % 2 === 1 ? 1 : 0);
  return (
    <g transform={me.facing === 'right' ? `translate(${2 * box.x + box.w} 0) scale(-1 1)` : undefined} className="pointer-events-none">
      <svg x={box.x} y={box.y} width={box.w} height={box.h} viewBox={`${col * CELL[0]} 0 ${CELL[0]} ${CELL[1]}`}>
        <image href={FOUNDER_SHEET} width={SIZE[0]} height={SIZE[1]} />
      </svg>
    </g>
  );
}

/** The founder in the city, `tall` tiles tall at `tile` pixels a tile; its spot is in tiles. */
export function CityFounder({ me, tile, tall, stride }: { me: Walker; tile: number; tall: number; stride: number }) {
  return <FounderSprite me={me} box={boxAt(me.at[0] * tile, me.at[1] * tile, (tall * tile) / CELL[1])} stride={stride} />;
}

/** The founder in an office, with whatever of the room stands nearer laid over it. */
export function FounderDrawing({ me, ground }: { me: Walker; ground: Ground }) {
  const [x, z] = me.at;
  const [fx, fy] = ground.toPixel(x, 0, z);
  const box = boxAt(fx, fy, SCALE);
  // The overlay only changes when the founder moves a whole pixel.
  const key = `${Math.round(box.x)},${Math.round(box.y)}`;
  const front = useFront(ground, box, key, ground.depthOf(x, 0.45, z));
  return (
    <g className="pointer-events-none">
      <FounderSprite me={me} box={box} stride={ground.stride} />
      {front && <image href={front.url} x={front.x} y={front.y} width={front.w} height={front.h} style={{ imageRendering: 'pixelated' }} />}
    </g>
  );
}

function useFront(ground: Ground, box: Box, key: string, depth: number): Figure['front'] {
  const { pic, kinds } = ground;
  // eslint-disable-next-line react-hooks/exhaustive-deps -- box and depth change with key
  return useMemo(() => frontOf(pic, box, (i) => !FLAT.has(kinds[pic.id[i]] ?? '') && pic.depth[i] < depth), [key, ground]);
}
