// A figure walking on a drawn room's floor (the founder, or an agent on its way somewhere): its frame from a sheet of walking frames, at
// the screen's own resolution, and the room's nearer pixels laid back over it in one small canvas as it moves.
import { useLayoutEffect, useMemo, useRef } from 'react';
import type { Ground } from '#src/town/founder.tsx';
import { NAME_HALO, NAME_INK } from '#src/town/palette.ts';
import { depthByRow, frontPixels, OVER_CHAIR, type Box } from '#src/town/people.ts';
import type { Spot } from '#src/town/walk.ts';
import { Monitors } from '#src/town/monitors.tsx';

/** A sheet of walking frames: its image, one cell's size and the whole size, in sheet pixels. */
export type Sheet = { url: string; cell: readonly [number, number]; size: readonly [number, number] };

/** The topmost picture row of the object tagged `tag`; null when the picture does not show it. */
function topOf(ground: Ground, tag: string): number | null {
  const { pic, tags } = ground;
  for (let i = 0; i < pic.id.length; i++) if (tags[pic.id[i]] === tag) return Math.floor(i / pic.w);
  return null;
}

/** The box of a cell of `sheet` with its feet at floor spot `at`; a cell keeps one empty row under the feet. */
export function boxOn(ground: Ground, sheet: Sheet, [x, z]: Spot): Box {
  const [fx, fy] = ground.toPixel(x, 0, z);
  const [w, h] = [sheet.cell[0] * ground.sprite, sheet.cell[1] * ground.sprite];
  return { x: fx - w / 2, y: fy + ground.sprite - h, w, h };
}

/** Cell (`col`, `row`) of `sheet` standing at `at`, mirrored when `flip`, with whatever of the room stands nearer laid over it; `label`
 *  is drawn under the feet, as each agent's name is. `sit` draws it sitting in `chair` (`head`: how far down its cell the head
 *  starts), nothing of it under a seat `seat` metres high, covered only by its chair. */
export function WalkingFigure({
  ground,
  sheet,
  at,
  col,
  row,
  flip,
  label,
  sit,
}: {
  ground: Ground;
  sheet: Sheet;
  at: Spot;
  col: number;
  row: number;
  flip: boolean;
  label?: { text: string; size: number; halo: number };
  sit?: { chair: string; head: number; seat: number };
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [x, z] = at;
  const [fx, fy] = ground.toPixel(x, 0, z);
  const stood = boxOn(ground, sheet, at);
  const top = useMemo(() => (sit ? topOf(ground, sit.chair) : null), [ground, sit]);
  // Seated, head and shoulders over the chair's back as a seated agent's are.
  const box = sit && top !== null ? { ...stood, y: top - OVER_CHAIR - stood.h * sit.head } : stood;
  // Seated, only what is over the seat is drawn.
  const shown = sit ? Math.max(0, Math.min(box.h, ground.toPixel(x, sit.seat, z)[1] - box.y)) : box.h;
  // The overlay is whole picture pixels around the figure.
  const [bx, by, bw, bh] = [Math.floor(box.x), Math.floor(box.y), Math.ceil(box.w) + 1, Math.ceil(box.h) + 1];
  // Before paint, so the figure never shows a frame without what stands in front of it.
  useLayoutEffect(() => {
    const c = canvas.current;
    if (!c) return;
    const { pic, under } = ground;
    [c.width, c.height] = [bw, bh];
    const depth = depthByRow(ground.toPixel, ground.depthOf, x, z);
    // A moving screen is laid over below as it moves, not copied still.
    const moving = new Set((ground.screens ?? []).map((s) => s.id));
    // Seated, only by its own chair: the desk and screen behind never do.
    const nearer = (i: number) => (sit ? ground.tags[pic.id[i]] === sit.chair : pic.depth[i] < depth(Math.floor(i / pic.w)));
    const covers = (i: number) => !under[pic.id[i]] && !moving.has(pic.id[i]) && nearer(i);
    const front = frontPixels(pic, { x: bx, y: by, w: bw, h: bh }, covers);
    if (front) c.getContext('2d')?.putImageData(front.img, front.x - bx, front.y - by);
  }, [ground, bx, by, bw, bh, x, z, sit]);
  const near = (ground.screens ?? []).filter((s) => {
    if (s.x >= bx + bw || s.x + s.w <= bx || s.y >= by + bh || s.y + s.h <= by) return false;
    const depth = depthByRow(ground.toPixel, ground.depthOf, x, z);
    return ground.pic.depth[s.pixel] < depth(Math.floor(s.pixel / ground.pic.w));
  });
  const [cw, ch] = sheet.cell;
  return (
    <g className="pointer-events-none">
      <g transform={flip ? `translate(${2 * box.x + box.w} 0) scale(-1 1)` : undefined}>
        <svg x={box.x} y={box.y} width={box.w} height={shown} viewBox={`${col * cw} ${row * ch} ${cw} ${(ch * shown) / box.h}`}>
          <image href={sheet.url} width={sheet.size[0]} height={sheet.size[1]} />
        </svg>
      </g>
      <foreignObject x={bx} y={by} width={bw} height={bh}>
        <canvas ref={canvas} style={{ display: 'block', width: '100%', height: '100%', imageRendering: 'pixelated' }} />
      </foreignObject>
      {near.length > 0 && (
        <svg x={bx} y={by} width={bw} height={bh} viewBox={`${bx} ${by} ${bw} ${bh}`} overflow="hidden">
          <Monitors screens={near} />
        </svg>
      )}
      {label && (
        <text
          x={fx}
          y={fy + 1 + label.size}
          textAnchor="middle"
          fontSize={label.size}
          fontWeight={label.size < 9 ? 700 : 600}
          fill={NAME_INK}
          stroke={NAME_HALO}
          strokeWidth={label.halo}
          paintOrder="stroke"
        >
          {label.text}
        </text>
      )}
    </g>
  );
}
