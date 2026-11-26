// The people in an office's picture (Inside.tsx): each seated or standing where its place is, or walking to a new one (crowd.ts), and the
// founder among them, all drawn back to front so whoever is nearer covers whoever is behind; the names under them; the bubbles of those
// waiting for the founder.
import { useSyncExternalStore } from 'react';
import { useMovers, type Crowd, type Mover } from '#src/town/crowd.ts';
import { FounderDrawing, founderBox, useFounder, type Founder } from '#src/town/founder.tsx';
import type { Door } from '#src/town/moves.ts';
import { touches, type Drawn } from '#src/town/office.ts';
import { NAME_HALO, NAME_INK } from '#src/town/palette.ts';
import { BUBBLE, PEOPLE_SHEET, SHEET, WALK, WALK_SHEET, walkColumn, type Figure } from '#src/town/people.ts';
import { boxOn, WalkingFigure, type Sheet } from '#src/town/walking.tsx';

const PIXEL = { imageRendering: 'pixelated' } as const;
const WALKERS: Sheet = { url: WALK_SHEET, cell: WALK.cell, size: WALK.size };
/** Metres between the two feet frames. */
const STRIDE = 0.16;

export function People({ drawn, founder, crowd, scale }: { drawn: Drawn; founder: Founder; crowd: Crowd; scale: number }) {
  const movers = useMovers(crowd);
  const font = nameSize(scale);
  const items = [
    ...drawn.cast.figures.filter((f) => !movers.has(f.person.key)).map((f) => ({ z: f.z, el: <Drawing key={f.person.key} figure={f} /> })),
    ...[...movers].map(([key, m]) => ({
      z: m.at[1],
      el: <Walking key={key} drawn={drawn} mover={m} label={{ text: m.name, size: font, halo: font * HALO }} />,
    })),
  ].sort((a, b) => a.z - b.z);
  // Those up to `behind` are behind the founder. One list, each keyed, so nobody is drawn anew as the founder walks past.
  const behind = useFounder(founder, (me) => (me ? items.filter((i) => i.z <= me.at[1]).length : 0));
  const names = drawn.cast.names
    .filter((n) => !movers.has(n.person.key))
    .map((n) => <Label key={`name:${n.person.key}`} x={n.x} y={n.y} font={font} text={n.person.name} />);
  return [
    ...items.slice(0, behind).map((i) => i.el),
    ...names,
    <FounderDrawing key="founder" founder={founder} ground={drawn.ground} />,
    ...items.slice(behind).map((i) => i.el),
  ];
}

function Walking({ drawn, mover: m, label }: { drawn: Drawn; mover: Mover; label: { text: string; size: number; halo: number } }) {
  return (
    <WalkingFigure
      ground={drawn.ground}
      sheet={WALKERS}
      at={m.at}
      col={walkColumn(m.facing, m.walked / STRIDE)}
      row={m.row}
      flip={m.facing === 'right'}
      label={label}
    />
  );
}

/** The bubbles over those waiting for the founder, once they sit. */
export function Bubbles({ drawn, crowd }: { drawn: Drawn; crowd: Crowd }) {
  const movers = useMovers(crowd);
  return drawn.cast.bubbles
    .filter((b) => !movers.has(b.key))
    .map((b) => <image key={b.key} href={BUBBLE} x={b.x} y={b.y} width={b.w} height={b.h} style={PIXEL} className="pointer-events-none" />);
}

/** Whether anyone, the founder or a person walking, reaches into the front wall, which then goes see-through. */
export function useSeeThrough(drawn: Drawn | null, founder: Founder, crowd: Crowd): boolean {
  const me = useFounder(founder, (me) => !!drawn && !!me && touches(drawn.front.top, founderBox(drawn.ground, me.at)));
  const them = useSyncExternalStore(crowd.subscribe, () =>
    [...crowd.get().values()].some((m) => !!drawn && touches(drawn.front.top, boxOn(drawn.ground, WALKERS, m.at))),
  );
  return !!drawn && (drawn.front.people || me || them);
}

/** How near the door one walking opens it: about half a second's walk. */
const NEAR = 0.5;

/** Which way someone near the door walks through it now: `in`, `out`, or nobody. */
export function useThroughDoor(crowd: Crowd, door: Door | undefined): 'in' | 'out' | null {
  return useSyncExternalStore(crowd.subscribe, () => {
    const near = (at: [number, number]) => !!door && Math.abs(at[0] - door.inside[0]) < NEAR && at[1] > door.inside[1] - NEAR;
    const m = [...crowd.get().values()].find((m) => near(m.at));
    return m ? (m.leaving ? 'out' : 'in') : null;
  });
}

/** A name's size in picture pixels: 11px on screen, smaller once the room is drawn too small for names that size to sit apart. */
export const nameSize = (scale: number) => Math.min(11 / scale, 6.5);
/** A name's halo, for its size. */
const HALO = 2.5 / 11;

/** A name under a person or desk, `font` picture pixels tall. */
export function Label({ x, y, font, text }: { x: number; y: number; font: number; text: string }) {
  return (
    <text
      x={x}
      y={y + font}
      textAnchor="middle"
      fontSize={font}
      fontWeight={600}
      fill={NAME_INK}
      stroke={NAME_HALO}
      strokeWidth={font * HALO}
      paintOrder="stroke"
      className="pointer-events-none"
    >
      {text}
    </text>
  );
}

/** A person at the screen's own resolution (smoothly scaled, never snapped to the room's pixels), then the room's nearer pixels over it. */
function Drawing({ figure: { box, col, row, front } }: { figure: Figure }) {
  const [cw, ch] = SHEET.cell;
  return (
    <g className="pointer-events-none">
      <svg x={box.x} y={box.y} width={box.w} height={box.h} viewBox={`${col * cw} ${row * ch} ${cw} ${ch}`}>
        <image href={PEOPLE_SHEET} width={SHEET.size[0]} height={SHEET.size[1]} />
      </svg>
      {front && <image href={front.url} x={front.x} y={front.y} width={front.w} height={front.h} style={PIXEL} />}
    </g>
  );
}
