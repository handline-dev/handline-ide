// A workspace's office, shown in place of the city when its building is clicked: a 3D room drawn as pixel art (room.ts lays it out,
// scene3d.ts and shade.ts draw it, people.ts seats the people), its benches as wide as fill the pane best and the picture filling it.
// The founder walks in it (founder.tsx): a click on the floor walks there, one on a person or a desk walks up to it and floats that
// agent's chat over the room (floater.tsx), one on the founder's own desk its decisions; the window stays wherever the founder walks
// until it is closed or another is clicked. The door
// the founder came in by shuts behind it, and opens when it walks up into it to go back out.
import { AnimatePresence } from 'framer-motion';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Decisions, WORDS } from '#src/town/panel.tsx';
import type { Decision, House, Person } from '#src/town/model.ts';
import { Chat } from '#src/chat/Chat.tsx';
import { Floater } from '#src/town/floater.tsx';
import { BUBBLE, PEOPLE_SHEET, SHEET, type Figure, type Spot } from '#src/town/people.ts';
import { COLS, roomOf } from '#src/town/room.ts';
import { viewOf } from '#src/town/view.ts';
import { NAME_HALO, NAME_INK, THROUGH } from '#src/town/palette.ts';
import { DoorSwing, type Way } from '#src/town/door.tsx';
import { FounderDrawing, founderBox, useFounder, useWalker } from '#src/town/founder.tsx';
import type { Spot as FloorSpot } from '#src/town/walk.ts';
import { touches, useDrawn } from '#src/town/office.ts';

const PIXEL = { imageRendering: 'pixelated' } as const;
/** The window of the founder's own desk, beside an agent's by its key. */
const MINE = 'mine';
/** How much of the front wall shows while a person reaches into it. */
const SEE_THROUGH = 0.4;

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
  // The door, pushed open into the room as the founder comes in, shuts behind it; walking down into it pushes it open out, toward the
  // viewer, and then goes back out to the city.
  const [swing, setSwing] = useState<{ way: Way; from?: FloorSpot; out?: boolean } | null>(cameIn ? { way: 'shut' } : null);
  const founder = useWalker(drawn?.ground ?? null, house.key, ([x, z], [, dz]) => {
    const entry = drawn?.ground.entry;
    if (!swing && entry && dz > 0 && Math.abs(x - entry[0]) < 0.35 && z > entry[1]) setSwing({ way: 'open', from: [x, z], out: true });
  });
  const cast = drawn?.cast;
  // Whose window floats: an agent's key, 'mine' for the founder's desk, or none; set by a click, never by walking past.
  const [shown, setShown] = useState<string | null>(null);
  // The front wall goes see-through while a person reaches into it.
  const seeThrough = useFounder(
    founder,
    (me) => !!drawn && (drawn.front.people || (!!me && touches(drawn.front.top, founderBox(drawn.ground, me.at)))),
  );
  // The people are in depth order: those up to `behind` stand behind the founder.
  const behind = useFounder(founder, (me) => (me && cast ? cast.figures.filter((f) => f.z <= me.at[1]).length : 0));
  const person = shown && shown !== MINE ? (house.people.find((p) => p.key === shown) ?? null) : null;
  const font = 11 / scale; // names stay 11px on screen whatever the scale
  const decisions = house.decisions.length;

  useEffect(() => {
    // Escape the chat took (to stop a turn, close a list) stays the chat's; else it closes the window, then leaves the office.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented || (e.target instanceof Element && e.target.closest('[data-no-walk]'))) return;
      if (shown) setShown(null);
      else onBack();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onBack, shown]);

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
      // Whose window is open stays outlined, wherever the founder walks.
      className={shown === s.person.key ? 'cursor-pointer stroke-accent' : 'cursor-pointer hover:stroke-accent'}
      onClick={() => {
        founder.go(s.at);
        setShown(s.person.key);
      }}
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
              <g opacity={seeThrough ? SEE_THROUGH : 1} style={{ transition: 'opacity 150ms' }} className="pointer-events-none">
                <image href={drawn.front.url} width={drawn.w} height={drawn.h} style={PIXEL} />
                {drawn.front.door && drawn.leaves && swing && (
                  <DoorSwing
                    pic={drawn.front.pic}
                    door={{ box: drawn.front.door, through: THROUGH, leaves: drawn.leaves }}
                    toPixel={drawn.ground.toPixel}
                    way={swing.way}
                    out={swing.out}
                    onDone={() => {
                      if (swing.way !== 'open') return setSwing(null);
                      // Sent somewhere else while it opened: it shuts again.
                      const [me, from] = [founder.get()?.at, swing.from];
                      if (me && from && Math.hypot(me[0] - from[0], me[1] - from[1]) < 0.05) onBack();
                      else setSwing({ way: 'shut', out: true });
                    }}
                  />
                )}
              </g>
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
                    className={shown === MINE ? 'cursor-pointer stroke-accent' : 'cursor-pointer hover:stroke-accent'}
                    onClick={() => {
                      if (!drawn.cast.mine) return;
                      founder.go(drawn.cast.mine.at);
                      setShown(MINE);
                    }}
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
        <AnimatePresence>
          {person && (
            <Floater key="window" title={person.name} sub={person.doing ?? WORDS[person.activity]} onClose={() => setShown(null)}>
              <Chat key={person.key} agent={person.key} />
            </Floater>
          )}
          {shown === MINE && (
            <Floater
              key="window"
              title="Your desk"
              sub={`${decisions} ${decisions === 1 ? 'decision' : 'decisions'}`}
              onClose={() => setShown(null)}
            >
              <div className="min-h-0 flex-1 overflow-y-auto">
                <Decisions papers={house.decisions} onOpen={onOpen} />
              </div>
            </Floater>
          )}
        </AnimatePresence>
      </div>
    </div>
  );
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
