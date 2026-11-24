// The founder in an office, from public/town/founder.png: standing and walking, facing down, up and left (right is left mirrored). The
// founder comes in at the door and walks where a click sends it, or where the arrow keys (or WASD) push it; the room's nearer pixels are
// laid over it, as over the seated people.
import { useEffect, useMemo, useRef, useState } from 'react';
import { frontOf, SCALE, type Box, type Figure } from '#src/town/people.ts';
import type { Picture } from '#src/town/shade.ts';
import { freeAt, nearestFree, pathTo, step, type Floor, type Spot } from '#src/town/walk.ts';

export const FOUNDER_SHEET = `${import.meta.env.BASE_URL}town/founder.png`;
const CELL = [136, 226] as const;
const SIZE = [816, 226] as const;
const SPEED = 1.4; // metres a second
const STRIDE = 0.16; // metres between the two feet frames
const FLAT = new Set(['floor', 'baseboard', 'rugRectangle']);

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

/** What the founder's drawing needs of the room: its pixels and objects, and where a floor spot or a point lands in the picture. */
export type Ground = {
  pic: Picture;
  kinds: string[];
  floor: Floor;
  entry: Spot;
  toPixel: (x: number, y: number, z: number) => [number, number];
  /** The floor spot drawn at picture pixel (px, py). */
  toFloor: (px: number, py: number) => Spot;
  depthOf: (x: number, y: number, z: number) => number;
};
export type Walker = { at: Spot; facing: Facing; walked: number; moving: boolean };

const facingOf = ([dx, dz]: Spot, was: Facing): Facing =>
  Math.abs(dx) < 1e-6 && Math.abs(dz) < 1e-6 ? was : Math.abs(dx) > Math.abs(dz) ? (dx < 0 ? 'left' : 'right') : dz < 0 ? 'up' : 'down';
const typing = (t: EventTarget | null) => t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));

/** Where the founder last stood, so a view drawn again (a pane split beside it) keeps it there; entering another house starts at its door. */
let kept: { house: string; me: Walker } | null = null;

/** The founder's place and walk in `house`: `go` walks to a floor spot and then does `then`; held keys walk as they point. */
export function useWalker(ground: Ground | null, house: string) {
  const [moved, setMe] = useState<Walker | null>(() => (kept?.house === house ? kept.me : null));
  useEffect(() => {
    if (moved) kept = { house, me: moved };
  }, [moved, house]);
  // The founder stands inside the door until it first moves.
  const me = moved ?? (ground ? { at: ground.entry, facing: 'down' as const, walked: 0, moving: false } : null);
  const plan = useRef<{ path: Spot[]; then?: () => void } | null>(null);
  const held = useRef(new Set<string>());

  useEffect(() => {
    if (!ground) return;
    let last = performance.now();
    let frame = 0;
    const tick = (now: number) => {
      const dt = Math.min(0.05, (now - last) / 1000);
      last = now;
      setMe((was) => {
        let m = was ?? { at: ground.entry, facing: 'down' as const, walked: 0, moving: false };
        // The room laid out again (another bench width) may have put furniture where the founder stood.
        if (!freeAt(ground.floor, m.at)) m = { ...m, at: nearestFree(ground.floor, m.at) ?? ground.entry };
        const keys = [...held.current].map((k) => KEYS[k]).filter(Boolean);
        if (keys.length) {
          plan.current = null;
          const dir: Spot = [keys.reduce((n, k) => n + k[0], 0), keys.reduce((n, k) => n + k[1], 0)];
          const at = step(ground.floor, m.at, dir, SPEED * dt);
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
        const move = SPEED * dt;
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

/** The founder drawn at its spot, with whatever of the room stands nearer laid over it. */
export function FounderDrawing({ me, ground }: { me: Walker; ground: Ground }) {
  const [x, z] = me.at;
  const [fx, fy] = ground.toPixel(x, 0, z);
  const [w, h] = [CELL[0] * SCALE, CELL[1] * SCALE];
  const box: Box = { x: fx - w / 2, y: fy + SCALE - h, w, h };
  const col = COL[me.facing] + (me.moving && Math.floor(me.walked / STRIDE) % 2 === 1 ? 1 : 0);
  // The overlay only changes when the founder moves a whole pixel.
  const key = `${Math.round(box.x)},${Math.round(box.y)}`;
  const front = useFront(ground, box, key, ground.depthOf(x, 0.45, z));
  return (
    <g className="pointer-events-none">
      <g transform={me.facing === 'right' ? `translate(${2 * fx} 0) scale(-1 1)` : undefined}>
        <svg x={box.x} y={box.y} width={box.w} height={box.h} viewBox={`${col * CELL[0]} 0 ${CELL[0]} ${CELL[1]}`}>
          <image href={FOUNDER_SHEET} width={SIZE[0]} height={SIZE[1]} />
        </svg>
      </g>
      {front && <image href={front.url} x={front.x} y={front.y} width={front.w} height={front.h} style={{ imageRendering: 'pixelated' }} />}
    </g>
  );
}

function useFront(ground: Ground, box: Box, key: string, depth: number): Figure['front'] {
  const { pic, kinds } = ground;
  // eslint-disable-next-line react-hooks/exhaustive-deps -- box and depth change with key
  return useMemo(() => frontOf(pic, box, (i) => !FLAT.has(kinds[pic.id[i]] ?? '') && pic.depth[i] < depth), [key, ground]);
}
