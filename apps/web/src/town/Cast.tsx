// The people in an office's picture (Inside.tsx): each seated or standing where its place is, or walking to a new one (crowd.ts), and the
// founder among them, all drawn back to front so whoever is nearer covers whoever is behind; the names under them; the bubbles of those
// waiting for the founder. One seated moves by what it is doing (motion.ts), from the newest people (MotionsOf), so the room is not drawn
// again.
import { useRead } from '@handline/app-data';
import { use, useEffect, useState, useSyncExternalStore } from 'react';
import { talking, useMovers, type Crowd, type Mover } from '#src/town/crowd.ts';
import { FounderDrawing, founderBox, useFounder, type Founder } from '#src/town/founder.tsx';
import type { Door } from '#src/town/moves.ts';
import { lineUp } from '#src/town/names.ts';
import { touches, type Drawn } from '#src/town/office.ts';
import { NAME_HALO, NAME_INK } from '#src/town/palette.ts';
import { BUBBLE, walkColumn, type Cast, type Figure } from '#src/town/people.ts';
import { hashOf, PEOPLE_SHEET, SHEET } from '#src/town/sheet.ts';
import { outlineDots } from '#src/town/outline.ts';
import { boxOn, WalkingFigure } from '#src/town/walking.tsx';
import { WALKERS } from '#src/town/look.ts';
import { useElapsed, type Cheering } from '#src/town/cheers.ts';
import { hop } from '#src/town/Cheer.tsx';
import { useLookIndex } from '#src/town/look.ts';
import { MOTION_CSS, motionStyle, type Motion } from '#src/town/motion.ts';
import { MotionsOf } from '#src/town/monitors.tsx';

const PIXEL = { imageRendering: 'pixelated' } as const;
/** Metres between the two feet frames. */
const STRIDE = 0.16;

export function People({
  drawn,
  founder,
  crowd,
  scale,
  cheers,
}: {
  drawn: Drawn;
  founder: Founder;
  crowd: Crowd;
  scale: number;
  /** Those celebrating a finished task, who hop where they are. */
  cheers: Cheering;
}) {
  const movers = useMovers(crowd);
  const font = nameSize(scale);
  const motions = use(MotionsOf);
  const items = [
    ...drawn.cast.figures
      .filter((f) => !movers.has(f.person.key))
      .map((f) => ({
        z: f.z,
        el: <Drawing key={f.person.key} figure={f} cheer={cheers.get(f.person.key)} motion={motions.get(f.person.key) ?? null} />,
      })),
    ...[...movers].map(([key, m]) => ({
      z: m.at[1],
      el: <Walking key={key} drawn={drawn} mover={m} label={{ text: m.name, size: font, halo: font * HALO }} />,
    })),
  ].sort((a, b) => a.z - b.z);
  // Those up to `behind` are behind the founder. One list, each keyed, so nobody is drawn anew as the founder walks past.
  const behind = useFounder(founder, (me) => (me ? items.filter((i) => i.z <= me.at[1]).length : 0));
  const { hire, mine } = drawn.cast;
  const labels = [
    ...drawn.cast.names
      .filter((n) => !movers.has(n.person.key))
      .map((n) => ({ item: { key: `name:${n.person.key}`, fill: NAME_INK }, x: n.x, y: n.y, text: n.person.name })),
    // In the accent colour: something to do, not somebody.
    ...(hire ? [{ item: { key: 'hire', fill: 'var(--accent)' }, x: hire.name[0], y: hire.name[1], text: '+ Hire' }] : []),
    ...(mine ? [{ item: { key: 'you', fill: NAME_INK }, x: mine.name[0], y: mine.name[1], text: 'You' }] : []),
  ];
  const names = lineUp(labels, font, (t) => textWidth(t, font)).map((l) => (
    <Label key={l.item.key} x={l.x} y={l.y} font={font} text={l.text} fill={l.item.fill} />
  ));
  return [
    <style key="motion">{MOTION_CSS}</style>,
    ...items.slice(0, behind).map((i) => i.el),
    ...names,
    <FounderDrawing key="founder" founder={founder} ground={drawn.ground} />,
    ...items.slice(behind).map((i) => i.el),
  ];
}

function Walking({ drawn, mover: m, label }: { drawn: Drawn; mover: Mover; label: { text: string; size: number; halo: number } }) {
  const box = boxOn(drawn.ground, WALKERS, m.at);
  return (
    <>
      <WalkingFigure
        ground={drawn.ground}
        sheet={WALKERS}
        at={m.at}
        col={walkColumn(m.facing, m.walked / STRIDE)}
        row={m.row}
        flip={m.facing === 'right'}
        label={label}
      />
      {talking(m) && <Speech x={m.facing === 'left' ? box.x + box.w * 0.18 : box.x + box.w * 0.82} y={box.y + box.h * 0.04} />}
    </>
  );
}

/** A speech bubble with its tail at (x, y), over the head of one giving work, a sheet of paper in it. */
function Speech({ x, y }: { x: number; y: number }) {
  const [w, h] = [15, 9];
  return (
    <g className="pointer-events-none" transform={`translate(${x - w / 2} ${y - h - 3})`}>
      <path
        d={`M2 0H${w - 2}Q${w} 0 ${w} 2V${h - 2}Q${w} ${h} ${w - 2} ${h}H${w / 2 + 2}L${w / 2} ${h + 3}L${w / 2 - 2} ${h}H2Q0 ${h} 0 ${h - 2}V2Q0 0 2 0Z`}
        fill={NAME_HALO}
        stroke={NAME_INK}
        strokeWidth={0.8}
      />
      {/* A sheet of paper: the work being given, as the papers on a desk are. */}
      <rect x={w / 2 - 2.5} y={1.5} width={5} height={6} fill="white" stroke={NAME_INK} strokeWidth={0.6} />
      {[3.4, 4.9].map((y) => (
        <line key={y} x1={w / 2 - 1.5} x2={w / 2 + 1.5} y1={y} y2={y} stroke={NAME_INK} strokeWidth={0.5} />
      ))}
    </g>
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
  // The founder's box is its look's: another look is looked at again.
  useLookIndex();
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

/** A name's size in picture pixels: 11px on screen. */
export const nameSize = (scale: number) => 11 / scale;
/** The least an office is drawn at, so its names are never more than 6.5 picture pixels, what a bench's desks have room for. */
export const READABLE = 11 / 6.5;
let measurer: { ctx: CanvasRenderingContext2D | null; family: string } | null = null;
/** How wide a name is drawn at `font` picture pixels. */
function textWidth(text: string, font: number): number {
  measurer ??= { ctx: document.createElement('canvas').getContext('2d'), family: getComputedStyle(document.body).fontFamily };
  if (!measurer.ctx) return text.length * font * 0.62;
  measurer.ctx.font = `600 ${font}px ${measurer.family}`;
  return measurer.ctx.measureText(text).width;
}

/** A name's halo, for its size. */
const HALO = 2.5 / 11;

/** A name under a person or desk, `font` picture pixels tall, in the names' ink unless `fill` gives another. */
export function Label({ x, y, font, text, fill = NAME_INK }: { x: number; y: number; font: number; text: string; fill?: string }) {
  return (
    <text
      x={x}
      y={y + font}
      textAnchor="middle"
      fontSize={font}
      fontWeight={600}
      fill={fill}
      stroke={NAME_HALO}
      strokeWidth={font * HALO}
      paintOrder="stroke"
      className="pointer-events-none"
    >
      {text}
    </text>
  );
}

/** Seconds into its motion a person starts, the same for the same person, so a room does not move in step. */
const offsetOf = (key: string) => (hashOf(key) % 1000) / 250;

/** A person at the screen's own resolution (smoothly scaled, never snapped to the room's pixels), moving as `motion` says, then the
 *  room's nearer pixels over it; while it cheers (`cheer`: when that began) it hops instead, its chair staying put. */
function Drawing({
  figure: { box, col, row, front, person, neck },
  cheer,
  motion,
}: {
  figure: Figure;
  cheer?: number;
  motion: Motion | null;
}) {
  const [cw, ch] = SHEET.cell;
  const skip = useElapsed(cheer);
  const moves = !cheer && motion && neck;
  return (
    <g className="pointer-events-none">
      <g
        className={moves ? 'town-motion' : undefined}
        style={cheer ? hop(skip) : moves ? motionStyle(motion, neck, offsetOf(person.key)) : undefined}
      >
        <svg x={box.x} y={box.y} width={box.w} height={box.h} viewBox={`${col * cw} ${row * ch} ${cw} ${ch}`}>
          <image href={PEOPLE_SHEET} width={SHEET.size[0]} height={SHEET.size[1]} />
        </svg>
      </g>
      {front && <image href={front.url} x={front.x} y={front.y} width={front.w} height={front.h} style={PIXEL} />}
    </g>
  );
}

/** The dotted outline's grid, in sheet pixels: a dot is a cell of it. */
const DOT = 7;
/** Each figure's dotted outline as an image, by its sheet row, once made. */
const outlines = new Map<number, Promise<string>>();

/** A figure's dotted outline, drawn from the sheet's walking-down pose (its third column) once it has loaded. */
function outlineOf(row: number): Promise<string> {
  const made = outlines.get(row);
  if (made) return made;
  const url = new Promise<string>((resolve, reject) => {
    const sheet = new Image();
    const fail = (e: unknown) => {
      outlines.delete(row);
      reject(e instanceof Error ? e : new Error('the people sheet did not load'));
    };
    sheet.onerror = fail;
    sheet.onload = () => {
      try {
        resolve(dotted(sheet, row));
      } catch (e) {
        fail(e);
      }
    };
    sheet.src = PEOPLE_SHEET;
  });
  outlines.set(row, url);
  return url;
}

/** The dotted outline of the figure in `row` of the loaded sheet, as an image url. */
function dotted(sheet: HTMLImageElement, row: number): string {
  const [cw, ch] = SHEET.cell;
  const read = Object.assign(document.createElement('canvas'), { width: cw, height: ch }).getContext('2d')!;
  read.drawImage(sheet, -2 * cw, -row * ch);
  const rgba = read.getImageData(0, 0, cw, ch).data;
  const alpha = Array.from({ length: cw * ch }, (_, i) => rgba[i * 4 + 3]);
  const out = Object.assign(document.createElement('canvas'), { width: Math.ceil(cw / DOT), height: Math.ceil(ch / DOT) });
  const pen = out.getContext('2d')!;
  pen.fillStyle = NAME_INK;
  for (const [x, y] of outlineDots(alpha, cw, ch, DOT)) pen.fillRect(x, y, 1, 1);
  return out.toDataURL();
}

/** The HQ's dotted person, where the next agent hired would stand; none while there is no team to hire into, as in Targets.tsx. */
export function JoinOutline({ join }: { join: NonNullable<Cast['join']> }) {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    outlineOf(join.row).then(
      (u) => live && setUrl(u),
      () => {},
    );
    return () => {
      live = false;
    };
  }, [join.row]);
  const teams = useRead('workspaces.list', {}).data?.workspaces.length ?? 0;
  if (!url || !teams) return null;
  const [cw, ch] = SHEET.cell;
  // The grid's last cells reach past the cell: the image is drawn that much bigger, at the sheet's scale.
  const [gw, gh] = [Math.ceil(cw / DOT) * DOT, Math.ceil(ch / DOT) * DOT];
  const k = join.cell.w / cw;
  return <image href={url} x={join.cell.x} y={join.cell.y} width={gw * k} height={gh * k} style={PIXEL} className="pointer-events-none" />;
}
