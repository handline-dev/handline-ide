// A workspace's office, shown in place of the city when its building is clicked: a 3D room drawn as pixel art (room.ts lays it out,
// scene3d.ts and shade.ts draw it, people.ts seats the people), its benches as wide as fill the pane best and the picture filling it.
// The founder walks in it (founder.tsx): a click on the floor walks there, one on a person or a desk walks up to it. Standing beside an
// agent floats its chat over the room (floater.tsx), beside the founder's own desk its decisions; walking away puts it away. The door
// the founder came in by shuts behind it, and opens when it walks up into it to go back out.
import { useEffect, useMemo, useRef, useState } from 'react';
import { WebGLRenderer } from 'three';
import { Decisions, WORDS } from '#src/town/panel.tsx';
import type { Decision, House, Person } from '#src/town/model.ts';
import { Chat } from '#src/chat/Chat.tsx';
import { Floater } from '#src/town/floater.tsx';
import { boxOf, BUBBLE, castOf, PEOPLE_SHEET, SHEET, type Box, type Cast, type Figure, type Spot } from '#src/town/people.ts';
import { COLS, DOOR, roomOf, type Room } from '#src/town/room.ts';
import { viewOf } from '#src/town/view.ts';
import { drawRoom } from '#src/town/scene3d.ts';
import { shade } from '#src/town/shade.ts';
import { NAME_HALO, NAME_INK, THROUGH } from '#src/town/palette.ts';
import { DoorSwing, leavesOf, type Leaves, type Way } from '#src/town/door.tsx';
import { FounderDrawing, useFounder, useWalker, type Ground } from '#src/town/founder.tsx';
import type { Spot as FloorSpot } from '#src/town/walk.ts';
import { groundOf } from '#src/town/ground.ts';

const PIXEL = { imageRendering: 'pixelated' } as const;

let renderer: WebGLRenderer | null = null;

type Drawn = { url: string; w: number; h: number; cast: Cast; ground: Ground; door: Box | null; leaves: Leaves | null };

/** What a drawing shows: the bench width and each person's name and state, so a poll that changes nothing draws nothing. */
const sigOf = (house: House, cols: number) =>
  [house.key, cols, house.decisions.length, ...house.people.map((p) => `${p.key}:${p.name}:${p.activity}:${p.queue}`)].join(' ');
/** The last drawings, by house, so an office is there at once when it comes back; the oldest goes past MOST. */
const drawings = new Map<string, { sig: string; drawn: Drawn }>();
const MOST = 4;

/** The room drawn with its people, as an image URL and where each person and desk is; laid out and drawn again only when what it shows
 *  changes, not on each poll. `cols` is null until the pane has a size. */
function useDrawn(house: House, cols: number | null): Drawn | null {
  const sig = cols === null ? null : sigOf(house, cols);
  const lay = () => ({ sig, house: house.key, room: cols === null ? null : roomOf(house, cols) });
  const [laid, setLaid] = useState(lay);
  if (laid.sig !== sig) setLaid(lay());
  return useMemo(() => drawOffice(laid), [laid]);
}

/** The office for a layout: the kept drawing when it shows the same, else drawn now and kept. */
function drawOffice({ room, sig, house: key }: { room: Room | null; sig: string | null; house: string }): Drawn | null {
  const kept = drawings.get(key);
  if (!room || sig === null || kept?.sig === sig) return kept?.drawn ?? null;
  renderer ??= new WebGLRenderer({ antialias: false, alpha: true });
  const frame = drawRoom(renderer, room);
  const pic = shade(frame);
  const cast = castOf(pic, frame, room);
  const canvas = document.createElement('canvas');
  [canvas.width, canvas.height] = [pic.w, pic.h];
  canvas.getContext('2d')?.putImageData(new ImageData(pic.rgba, pic.w, pic.h), 0, 0);
  const ground = groundOf(pic, frame, room, { speed: 1.4, stride: 0.16 });
  const drawn = {
    url: canvas.toDataURL(),
    w: pic.w,
    h: pic.h,
    cast,
    ground,
    door: boxOf(pic, frame, DOOR),
    // Hinged by the corner, it opens out of the room.
    leaves: leavesOf(room.blocks, DOOR, 'right'),
  };
  drawings.delete(key);
  drawings.set(key, { sig, drawn });
  for (const old of drawings.keys()) if (drawings.size > MOST) drawings.delete(old);
  return drawn;
}

/** The bench width whose room fills a pane of (w, h) the most. */
function colsFor(house: House, w: number, h: number): number {
  let best = { cols: COLS[0], fit: 0 };
  for (let cols = COLS[0]; cols <= COLS[1]; cols++) {
    const v = viewOf(roomOf(house, cols));
    const fit = Math.min(w / v.w, h / v.h);
    if (fit > best.fit * 1.02) best = { cols, fit };
  }
  return best.cols;
}

/** The pane's size, kept as it resizes. */
function useSize(): [React.RefObject<HTMLDivElement | null>, [number, number]] {
  const ref = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState<[number, number]>([0, 0]);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const seen = new ResizeObserver(([e]) => setSize([e.contentRect.width, e.contentRect.height]));
    seen.observe(el);
    return () => seen.disconnect();
  }, []);
  return [ref, size];
}

const tipOf = (p: Person) => `${p.name} · ${p.doing ?? WORDS[p.activity]}`;

export function Inside({
  house,
  cameIn,
  onBack,
  onOpen,
  onMail,
}: {
  house: House;
  /** The founder walked in by the door, which then shuts behind it. */
  cameIn: boolean;
  onBack: () => void;
  onOpen: (d: Decision) => void;
  onMail: () => void;
}) {
  const [pane, [pw, ph]] = useSize();
  const cols = useMemo(() => (pw > 0 && ph > 0 ? colsFor(house, pw, ph) : null), [house, pw, ph]);
  const drawn = useDrawn(house, cols);
  const scale = drawn ? Math.min(pw / drawn.w, ph / drawn.h) : 1;
  // The door, open as the founder comes in, shuts behind it; walking up into it opens it, and then goes back out to the city.
  const [swing, setSwing] = useState<{ way: Way; from?: FloorSpot } | null>(cameIn ? { way: 'shut' } : null);
  const founder = useWalker(drawn?.ground ?? null, house.key, ([x, z], [, dz]) => {
    const entry = drawn?.ground.entry;
    if (!swing && entry && dz < 0 && Math.abs(x - entry[0]) < 0.35 && z < entry[1]) setSwing({ way: 'open', from: [x, z] });
  });
  const cast = drawn?.cast;
  const near = useFounder(founder, (me) => (me && cast ? nearOf(cast, me.at) : null));
  // The people are in depth order: those up to `behind` stand behind the founder.
  const behind = useFounder(founder, (me) => (me && cast ? cast.figures.filter((f) => f.z <= me.at[1]).length : 0));
  const nearPerson = near && near !== 'mine' ? (house.people.find((p) => p.key === near) ?? null) : null;
  const font = 11 / scale; // names stay 11px on screen whatever the scale
  const decisions = house.decisions.length;

  useEffect(() => {
    // Escape the chat took (to stop a turn, close a list) stays the chat's.
    const onKey = (e: KeyboardEvent) =>
      e.key === 'Escape' && !e.defaultPrevented && !(e.target instanceof Element && e.target.closest('[data-no-walk]')) && onBack();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onBack]);

  const hit = (s: Spot, key: string) => (
    <rect
      key={key}
      x={s.x - 1}
      y={s.y - 1}
      width={s.w + 2}
      height={s.h + 2}
      rx={2}
      fill="transparent"
      stroke="transparent"
      strokeWidth={1 / scale}
      className="cursor-pointer hover:stroke-accent"
      onClick={() => founder.go(s.at)}
    >
      <title>{tipOf(house.people.find((p) => p.key === s.person.key) ?? s.person)}</title>
    </rect>
  );

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-2 text-sm">
        <button type="button" onClick={onBack} className="rounded-md px-1.5 py-0.5 text-muted hover:bg-hover hover:text-fg">
          ← Town
        </button>
        <span className="truncate font-medium">{house.name}</span>
        <span className="truncate text-xs text-muted">
          {house.working ? `${house.working} of ${house.people.length} at work` : 'Lights off'}
        </span>
        <span className="flex-1" />
        <button
          type="button"
          onClick={onMail}
          className="flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-muted hover:bg-hover hover:text-fg"
        >
          Decisions
          {decisions > 0 && <span className="rounded-full bg-danger px-1.5 text-xs font-medium text-accent-fg">{decisions}</span>}
        </button>
      </div>
      <div className="relative flex min-h-0 flex-1 p-4">
        <div ref={pane} className="flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-hidden">
          {drawn && pw > 0 && (
            <svg
              width={drawn.w * scale}
              height={drawn.h * scale}
              viewBox={`0 0 ${drawn.w} ${drawn.h}`}
              className="shrink-0"
              role="img"
              aria-label={`${house.name}'s office`}
            >
              <image
                href={drawn.url}
                width={drawn.w}
                height={drawn.h}
                style={PIXEL}
                onClick={(e) => {
                  const r = e.currentTarget.getBoundingClientRect();
                  founder.go(drawn.ground.toFloor((e.clientX - r.left) / scale, (e.clientY - r.top) / scale));
                }}
              />
              {drawn.door && drawn.leaves && swing && (
                <DoorSwing
                  pic={drawn.ground.pic}
                  door={{ box: drawn.door, through: THROUGH, leaves: drawn.leaves }}
                  toPixel={drawn.ground.toPixel}
                  way={swing.way}
                  onDone={() => {
                    if (swing.way !== 'open') return setSwing(null);
                    // Sent somewhere else while it opened: it shuts again.
                    const [me, from] = [founder.get()?.at, swing.from];
                    if (me && from && Math.hypot(me[0] - from[0], me[1] - from[1]) < 0.05) onBack();
                    else setSwing({ way: 'shut' });
                  }}
                />
              )}
              {drawn.cast.figures.slice(0, behind).map((f) => (
                <Drawing key={f.person.key} figure={f} />
              ))}
              {drawn.cast.names.map((n) => (
                <Label key={n.person.key} x={n.x} y={n.y} font={font} scale={scale} text={n.person.name} />
              ))}
              <FounderDrawing founder={founder} ground={drawn.ground} />
              {drawn.cast.figures.slice(behind).map((f) => (
                <Drawing key={f.person.key} figure={f} />
              ))}
              {drawn.cast.bubbles.map((b) => (
                <image
                  key={`${b.x},${b.y}`}
                  href={BUBBLE}
                  x={b.x}
                  y={b.y}
                  width={b.w}
                  height={b.h}
                  style={PIXEL}
                  className="pointer-events-none"
                />
              ))}
              {drawn.cast.mine && (
                <>
                  <Label x={drawn.cast.mine.name[0]} y={drawn.cast.mine.name[1]} font={font} scale={scale} text="You" />
                  <rect
                    x={drawn.cast.mine.x - 1}
                    y={drawn.cast.mine.y - 1}
                    width={drawn.cast.mine.w + 2}
                    height={drawn.cast.mine.h + 2}
                    rx={2}
                    fill="transparent"
                    stroke="transparent"
                    strokeWidth={1 / scale}
                    className="cursor-pointer hover:stroke-accent"
                    onClick={() => drawn.cast.mine && founder.go(drawn.cast.mine.at)}
                  >
                    <title>{`Your desk · ${decisions} ${decisions === 1 ? 'decision' : 'decisions'}`}</title>
                  </rect>
                </>
              )}
              {drawn.cast.desks.map((d) => hit(d, `desk-${d.person.key}`))}
              {drawn.cast.spots.map((s) => hit(s, `at-${s.person.key}`))}
            </svg>
          )}
        </div>
        {nearPerson && (
          <Floater key={nearPerson.key} title={nearPerson.name} sub={nearPerson.doing ?? WORDS[nearPerson.activity]}>
            <Chat agent={nearPerson.key} />
          </Floater>
        )}
        {near === 'mine' && (
          <Floater title="Your desk" sub={`${decisions} ${decisions === 1 ? 'decision' : 'decisions'}`}>
            <div className="min-h-0 flex-1 overflow-y-auto">
              <Decisions papers={house.decisions} onOpen={onOpen} />
            </div>
          </Floater>
        )}
      </div>
    </div>
  );
}

/** How close the founder stands to an agent or its desk, in metres, for its chat to float. */
const NEAR = 0.75;

/** Whose the founder stands beside: an agent's key, 'mine' for the founder's own desk, or null; the nearest within NEAR. */
function nearOf(cast: Cast, [x, z]: FloorSpot): string | 'mine' | null {
  const all: [string, [number, number]][] = [
    ...cast.spots.map((s): [string, [number, number]] => [s.person.key, s.at]),
    ...cast.desks.map((s): [string, [number, number]] => [s.person.key, s.at]),
    ...(cast.mine ? [['mine', cast.mine.at] as [string, [number, number]]] : []),
  ];
  let best: [string | null, number] = [null, NEAR];
  for (const [key, at] of all) {
    const d = Math.hypot(at[0] - x, at[1] - z);
    if (d < best[1]) best = [key, d];
  }
  return best[0];
}

/** A name under a person or desk: `font` and the halo keep their screen size whatever the scale. */
function Label({ x, y, font, scale, text }: { x: number; y: number; font: number; scale: number; text: string }) {
  return (
    <text
      x={x}
      y={y + font}
      textAnchor="middle"
      fontSize={font}
      fontWeight={600}
      fill={NAME_INK}
      stroke={NAME_HALO}
      strokeWidth={2.5 / scale}
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
