// A workspace's office, shown in place of the city when its building is clicked: a 3D room drawn as pixel art (room.ts lays it out,
// scene3d.ts and shade.ts draw it, people.ts seats the people), its benches as wide as fill the pane best and the picture filling it.
// The founder walks in it (founder.tsx): a click on the floor walks there, one on a person or a desk walks up to it and brings up that
// agent's chat in the town's windows (Windows.tsx), one on the founder's own desk the decisions; whose chat a tab bar shows stays
// outlined. The door the founder came in by shuts behind it, and opens when it walks up into it to go back out. A person whose place
// changes walks to the new one (crowd.ts, Cast.tsx).
import { useEffect, useMemo, useRef, useState } from 'react';
import { WORDS, type House, type Person } from '#src/town/model.ts';
import { useTownShown } from '#src/town/Windows.tsx';
import type { Spot } from '#src/town/people.ts';
import { COLS, roomOf } from '#src/town/room.ts';
import { viewOf } from '#src/town/view.ts';
import { THROUGH } from '#src/town/palette.ts';
import { DoorSwing, type Way } from '#src/town/door.tsx';
import { useWalker } from '#src/town/founder.tsx';
import type { Spot as FloorSpot } from '#src/town/walk.ts';
import { useDrawn } from '#src/town/office.ts';
import { useCrowd } from '#src/town/crowd.ts';
import { Bubbles, Label, People, useSeeThrough, useThroughDoor } from '#src/town/Cast.tsx';

const PIXEL = { imageRendering: 'pixelated' } as const;
/** What the founder's own desk opens: the decisions waiting for them. */
const DESK = { opens: 'requests', title: 'Requests' };
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
}: {
  house: House;
  /** The founder walked in by the door, which then shuts behind it. */
  cameIn: boolean;
  onBack: () => void;
}) {
  const { show, shows } = useTownShown();
  const [pane, [pw, ph]] = useSize();
  const cols = useMemo(() => (pw > 0 && ph > 0 ? colsFor(house, pw, ph) : null), [house, pw, ph]);
  const drawn = useDrawn(house, cols);
  const scale = drawn ? Math.min(pw / drawn.w, ph / drawn.h) : 1;
  // The door, pushed open into the room as the founder comes in, shuts behind it; walking down into it pushes it open out, toward the
  // viewer, and then goes back out to the city. One of the people (`crowd`) coming or going holds it open while in the doorway.
  const [swing, setSwing] = useState<{ way: Way; from?: FloorSpot; out?: boolean; crowd?: boolean } | null>(
    cameIn ? { way: 'shut' } : null,
  );
  const founder = useWalker(drawn?.ground ?? null, house.key, ([x, z], [, dz]) => {
    const entry = drawn?.ground.entry;
    if (!swing && entry && dz > 0 && Math.abs(x - entry[0]) < 0.35 && z > entry[1]) setSwing({ way: 'open', from: [x, z], out: true });
  });
  const crowd = useCrowd(drawn?.stage ?? null);
  // The front wall goes see-through while a person reaches into it.
  const seeThrough = useSeeThrough(drawn, founder, crowd);
  // The door opens for whoever comes near it once it is free, and shuts when nobody is near.
  const through = useThroughDoor(crowd, drawn?.stage.door);
  if (through && (!swing || (swing.crowd && swing.way === 'shut'))) setSwing({ way: 'open', out: through === 'out', crowd: true });
  else if (!through && swing?.crowd && swing.way === 'open') setSwing({ way: 'shut', out: swing.out, crowd: true });
  const font = 11 / scale; // names stay 11px on screen whatever the scale
  const decisions = house.decisions.length;

  useEffect(() => {
    // Escape in a tab bar stays the tab's (to stop a turn, close a list); else it leaves the office.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented || (e.target instanceof Element && e.target.closest('[data-no-walk]'))) return;
      onBack();
    };
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
      // Whose chat a tab bar shows stays outlined, wherever the founder walks.
      className={shows(`chat:${s.person.key}`) ? 'cursor-pointer stroke-accent' : 'cursor-pointer hover:stroke-accent'}
      onClick={() => {
        founder.go(s.at);
        show(`chat:${s.person.key}`, s.person.name);
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
          onClick={() => show(DESK.opens, DESK.title)}
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
              <People drawn={drawn} founder={founder} crowd={crowd} scale={scale} />
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
                      if (swing.crowd && swing.way === 'open') return; // held open till they are through
                      if (swing.way !== 'open') return setSwing(null);
                      // Sent somewhere else while it opened: it shuts again.
                      const [me, from] = [founder.get()?.at, swing.from];
                      if (me && from && Math.hypot(me[0] - from[0], me[1] - from[1]) < 0.05) onBack();
                      else setSwing({ way: 'shut', out: true });
                    }}
                  />
                )}
              </g>
              <Bubbles drawn={drawn} crowd={crowd} />
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
                    className={shows(DESK.opens) ? 'cursor-pointer stroke-accent' : 'cursor-pointer hover:stroke-accent'}
                    onClick={() => {
                      if (!drawn.cast.mine) return;
                      founder.go(drawn.cast.mine.at);
                      show(DESK.opens, DESK.title);
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
      </div>
    </div>
  );
}
