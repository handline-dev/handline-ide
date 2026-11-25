// A door swinging, drawn over a room's picture (the city's or an office's) where the picture shows it shut: its own pixels, narrowing
// toward the hinge and darker as it turns away, with the doorway behind it. The picture itself is never drawn again for it.
import { useEffectEvent, useLayoutEffect, useRef } from 'react';
import type { Box } from '#src/town/people.ts';

/** Where a door is in a picture and what shows through it open; `double` doors part from the middle, hinged at both ends. */
export type Doorway = { box: Box; through: string; double?: boolean };
/** `open` swings it open, `shut` from open to shut, `held` shows it open; a new way swings again. */
export type Way = 'open' | 'shut' | 'held';

const MS = 380;
/** How wide a leaf still looks swung open: it turns away from the viewer, not edge-on. */
const OPEN = 0.2;

/** `onDone` hears a swing end. */
export function DoorSwing({
  url,
  size: [w, h],
  door,
  way,
  onDone,
}: {
  url: string;
  size: [number, number];
  door: Doorway;
  way: Way;
  onDone?: () => void;
}) {
  const leaves = useRef<(SVGGElement | null)[]>([]);
  const { box } = door;
  const halves: [number, number][] = door.double
    ? [
        [box.x, box.w / 2],
        [box.x + box.w / 2, box.w / 2],
      ]
    : [[box.x, box.w]];
  const done = useEffectEvent(() => onDone?.());
  useLayoutEffect(() => {
    const shut = { transform: 'scaleX(1)', filter: 'brightness(1)' };
    const open = { transform: `scaleX(${OPEN})`, filter: 'brightness(0.72)' };
    const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
    const runs = leaves.current.flatMap((g) =>
      g
        ? [
            g.animate(way === 'open' ? [shut, open] : way === 'shut' ? [open, shut] : [open, open], {
              duration: way === 'held' || still ? 0 : MS,
              easing: 'ease-in-out',
              fill: 'forwards',
            }),
          ]
        : [],
    );
    let live = true;
    void Promise.all(runs.map((r) => r.finished)).then(
      () => live && done(),
      () => undefined,
    );
    return () => {
      live = false;
      runs.forEach((r) => r.cancel());
    };
  }, [way]);
  return (
    <g className="pointer-events-none">
      <rect x={box.x} y={box.y} width={box.w} height={box.h} fill={door.through} />
      {halves.map(([x, lw], i) => {
        // The left leaf hinges at its left edge, the right one of a pair at its right.
        const hinge = door.double && i === 1 ? x + lw : x;
        return (
          <g
            key={i}
            ref={(g) => void (leaves.current[i] = g)}
            style={{ transformBox: 'view-box', transformOrigin: `${hinge}px ${box.y}px` }}
          >
            <svg x={x} y={box.y} width={lw} height={box.h} viewBox={`${x} ${box.y} ${lw} ${box.h}`}>
              <image href={url} width={w} height={h} style={{ imageRendering: 'pixelated' }} />
            </svg>
          </g>
        );
      })}
    </g>
  );
}
