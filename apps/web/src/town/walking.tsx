// A figure walking on a drawn room's floor (the founder, or an agent on its way somewhere): its frame from a sheet of walking frames, at
// the screen's own resolution, and the room's nearer pixels laid back over it in one small canvas as it moves.
import { useLayoutEffect, useRef } from 'react';
import type { Ground } from '#src/town/founder.tsx';
import { NAME_HALO, NAME_INK } from '#src/town/palette.ts';
import { depthByRow, frontPixels, type Box } from '#src/town/people.ts';
import type { Spot } from '#src/town/walk.ts';

/** A sheet of walking frames: its image, one cell's size and the whole size, in sheet pixels. */
export type Sheet = { url: string; cell: readonly [number, number]; size: readonly [number, number] };

/** The box of a cell of `sheet` with its feet at floor spot `at`; a cell keeps one empty row under the feet. */
export function boxOn(ground: Ground, sheet: Sheet, [x, z]: Spot): Box {
  const [fx, fy] = ground.toPixel(x, 0, z);
  const [w, h] = [sheet.cell[0] * ground.sprite, sheet.cell[1] * ground.sprite];
  return { x: fx - w / 2, y: fy + ground.sprite - h, w, h };
}

/** Cell (`col`, `row`) of `sheet` standing at `at`, mirrored when `flip`, with whatever of the room stands nearer laid over it; `label`
 *  is drawn under the feet, as each agent's name is. */
export function WalkingFigure({
  ground,
  sheet,
  at,
  col,
  row,
  flip,
  label,
}: {
  ground: Ground;
  sheet: Sheet;
  at: Spot;
  col: number;
  row: number;
  flip: boolean;
  label?: { text: string; size: number; halo: number };
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [x, z] = at;
  const box = boxOn(ground, sheet, at);
  const [fx, fy] = ground.toPixel(x, 0, z);
  // The overlay is whole picture pixels around the figure.
  const [bx, by, bw, bh] = [Math.floor(box.x), Math.floor(box.y), Math.ceil(box.w) + 1, Math.ceil(box.h) + 1];
  // Before paint, so the figure never shows a frame without what stands in front of it.
  useLayoutEffect(() => {
    const c = canvas.current;
    if (!c) return;
    const { pic, under } = ground;
    [c.width, c.height] = [bw, bh];
    const depth = depthByRow(ground.toPixel, ground.depthOf, x, z);
    const front = frontPixels(pic, { x: bx, y: by, w: bw, h: bh }, (i) => !under[pic.id[i]] && pic.depth[i] < depth(Math.floor(i / pic.w)));
    if (front) c.getContext('2d')?.putImageData(front.img, front.x - bx, front.y - by);
  }, [ground, bx, by, bw, bh, x, z]);
  const [cw, ch] = sheet.cell;
  return (
    <g className="pointer-events-none">
      <g transform={flip ? `translate(${2 * box.x + box.w} 0) scale(-1 1)` : undefined}>
        <svg x={box.x} y={box.y} width={box.w} height={box.h} viewBox={`${col * cw} ${row * ch} ${cw} ${ch}`}>
          <image href={sheet.url} width={sheet.size[0]} height={sheet.size[1]} />
        </svg>
      </g>
      <foreignObject x={bx} y={by} width={bw} height={bh}>
        <canvas ref={canvas} style={{ display: 'block', width: '100%', height: '100%', imageRendering: 'pixelated' }} />
      </foreignObject>
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
