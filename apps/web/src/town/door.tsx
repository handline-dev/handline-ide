// A door swinging, drawn over a room's picture (the city's or an office's) where the picture shows it shut: its own pixels turned about
// its hinge in the room, through the room's camera, and laid back on the picture's own pixel grid, darker as it turns from the light. A
// door opens away from the viewer, behind its frame: a building's into it, an office's out to the street. The picture itself is never
// drawn again for it.
import { useEffect, useEffectEvent, useMemo, useState } from 'react';
import type { Box } from '#src/town/people.ts';
import type { Block } from '#src/town/room.ts';
import type { Picture } from '#src/town/shade.ts';

/** A door in the room, in metres: its face at z, its height, and each leaf's span along x (two part from the middle, hinged at both
 *  ends; one hinges at its `hinge` end). */
export type Leaves = { z: number; h: number; spans: [number, number][]; hinge: 'left' | 'right' };
/** Where a door is in the picture, what shows through it open, and its leaves. */
export type Doorway = { box: Box; through: string; leaves: Leaves };
/** `open` swings it open, `shut` from open to shut, `held` shows it open; a new way swings again. */
export type Way = 'open' | 'shut' | 'held';

const MS = 450;
/** How far a leaf opens, in radians: short of a right angle, which the camera would see edge-on. */
const OPEN = (72 * Math.PI) / 180;
/** How much darker a leaf is turned fully from the light. */
const DIM = 0.25;

/** A door's leaves from the room's blocks with its tag: those standing on the floor (a pane set in a leaf does not). */
export function leavesOf(blocks: Block[], tag: string, hinge: Leaves['hinge']): Leaves | null {
  const on = blocks.filter((b) => b.tag === tag && b.from[1] === 0).sort((a, b) => a.from[0] - b.from[0]);
  if (on.length === 0) return null;
  return { z: on[0].to[2], h: on[0].to[1], spans: on.map((b) => [b.from[0], b.to[0]]), hinge };
}

const ease = (k: number) => (k < 0.5 ? 2 * k * k : 1 - (-2 * k + 2) ** 2 / 2);

/** How open a door is, 0 to 1, swinging `way`; `onDone` hears a swing end. */
function useOpen(way: Way, onDone: () => void): number {
  const [open, setOpen] = useState(way === 'open' ? 0 : 1);
  const done = useEffectEvent(onDone);
  useEffect(() => {
    const [from, to] = way === 'open' ? [0, 1] : way === 'shut' ? [1, 0] : [1, 1];
    const still = way === 'held' || matchMedia('(prefers-reduced-motion: reduce)').matches;
    const start = performance.now();
    let frame = 0;
    const tick = (now: number) => {
      const k = still ? 1 : Math.min(1, (now - start) / MS);
      setOpen(from + (to - from) * ease(k));
      if (k < 1) frame = requestAnimationFrame(tick);
      else done();
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [way]);
  return open;
}

/** A leaf in the picture: its pixel columns, the hinge's, and how a column `d` pixels from the hinge moves turned: to `a d` from the
 *  hinge and `b d` down. */
type Turn = { x0: number; x1: number; hinge: number; a: number; b: number };

/** The leaves turned, as pixels on the picture's grid inside the doorway: each pixel takes the shut leaf's pixel that lands on it. */
function turned(pic: Picture, box: Box, turns: Turn[], light: number): string {
  const out = new ImageData(box.w, box.h);
  for (let y = 0; y < box.h; y++)
    for (let x = 0; x < box.w; x++) {
      const [cx, cy] = [box.x + x + 0.5, box.y + y + 0.5];
      for (const t of turns) {
        const u = t.hinge + (cx - t.hinge) / t.a;
        const v = cy - t.b * (u - t.hinge);
        if (u < t.x0 || u >= t.x1 || v < box.y || v >= box.y + box.h) continue;
        const [from, to] = [(Math.floor(v) * pic.w + Math.floor(u)) * 4, (y * box.w + x) * 4];
        for (let c = 0; c < 3; c++) out.data[to + c] = pic.rgba[from + c] * light;
        out.data[to + 3] = pic.rgba[from + 3];
        break;
      }
    }
  const canvas = document.createElement('canvas');
  [canvas.width, canvas.height] = [box.w, box.h];
  canvas.getContext('2d')?.putImageData(out, 0, 0);
  return canvas.toDataURL();
}

export function DoorSwing({
  pic,
  door,
  toPixel,
  way,
  onDone,
}: {
  pic: Picture;
  door: Doorway;
  toPixel: (x: number, y: number, z: number) => [number, number];
  way: Way;
  onDone?: () => void;
}) {
  const angle = useOpen(way, () => onDone?.()) * OPEN;
  const { box, leaves } = door;
  const url = useMemo(() => {
    const double = leaves.spans.length > 1;
    const mid = Math.round(box.x + box.w / 2);
    const turns = leaves.spans.map(([x0, x1], i): Turn => {
      // A pair hinges at its outer ends; the pixels of each leaf are its half of the doorway's.
      const right = double ? i === 1 : leaves.hinge === 'right';
      const [hx, fx] = right ? [x1, x0] : [x0, x1];
      const [px0, px1] = double ? (i === 0 ? [box.x, mid] : [mid, box.x + box.w]) : [box.x, box.x + box.w];
      const width = Math.abs(fx - hx);
      const free = (a: number) => toPixel(hx + Math.sign(fx - hx) * Math.cos(a) * width, 0, leaves.z - Math.sin(a) * width);
      const [base, shut, open] = [toPixel(hx, 0, leaves.z), free(0), free(angle)];
      const span = shut[0] - base[0];
      return { x0: px0, x1: px1, hinge: right ? px1 : px0, a: (open[0] - base[0]) / span, b: (open[1] - shut[1]) / span };
    });
    return turned(pic, box, turns, 1 - DIM * Math.sin(angle));
  }, [pic, box, leaves, toPixel, angle]);
  return (
    <g className="pointer-events-none">
      <rect x={box.x} y={box.y} width={box.w} height={box.h} fill={door.through} />
      <image href={url} x={box.x} y={box.y} width={box.w} height={box.h} style={{ imageRendering: 'pixelated' }} />
    </g>
  );
}
