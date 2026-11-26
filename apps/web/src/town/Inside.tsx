// A workspace's office, shown in place of the city when its building is clicked: a 3D room drawn as pixel art (room.ts lays it out,
// scene3d.ts and shade.ts draw it, people.ts seats the people), the same room whatever the pane's size, drawn to fill it.
// The founder walks in it (founder.tsx): a click on the floor walks there, one on a person or a desk walks up to it and brings up that
// agent's chat in the town's windows (Windows.tsx), one on the papers on its desk its tasks, one on the founder's own desk the
// decisions; what a tab bar shows stays outlined. The door the founder came in by shuts behind it, and opens when it walks up into it or
// the door is clicked, to go back out; Escape never leaves. A person whose place changes walks to the new one (crowd.ts, Cast.tsx).
import { useEffect, useRef, useState } from 'react';
import { WORDS, type House, type Person } from '#src/town/model.ts';
import { useTownShown } from '#src/town/Windows.tsx';
import type { Spot } from '#src/town/people.ts';
import { THROUGH } from '#src/town/palette.ts';
import { DoorSwing, type Way } from '#src/town/door.tsx';
import { useWalker } from '#src/town/founder.tsx';
import type { Spot as FloorSpot } from '#src/town/walk.ts';
import { useDrawn } from '#src/town/office.ts';
import { useCrowd } from '#src/town/crowd.ts';
import { useGivenWork } from '#src/town/errands.ts';
import { Bubbles, Label, nameSize, People, useSeeThrough, useThroughDoor } from '#src/town/Cast.tsx';

const PIXEL = { imageRendering: 'pixelated' } as const;
/** What the founder's own desk opens: the decisions waiting for them. */
const DESK = { opens: 'requests', title: 'Requests' };
/** How much of the front wall shows while a person reaches into it. */
const SEE_THROUGH = 0.4;

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
  // The same room whatever the space: a smaller pane shows it smaller.
  const drawn = useDrawn(house);
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
  // One teammate giving another work walks over to it, talks and walks back.
  useGivenWork((from, to) => crowd.errand(from, to));
  // The front wall goes see-through while a person reaches into it.
  const seeThrough = useSeeThrough(drawn, founder, crowd);
  // The door opens for whoever comes near it once it is free, and shuts when nobody is near.
  const through = useThroughDoor(crowd, drawn?.stage.door);
  // Where the founder stands to go out once the door is free, after a click on it.
  const [leaving, setLeaving] = useState<FloorSpot | null>(null);
  if (leaving && (!swing || (swing.crowd && swing.way === 'shut'))) {
    setLeaving(null);
    setSwing({ way: 'open', from: leaving, out: true });
  } else if (through && (!swing || (swing.crowd && swing.way === 'shut'))) setSwing({ way: 'open', out: through === 'out', crowd: true });
  else if (!through && swing?.crowd && swing.way === 'open') setSwing({ way: 'shut', out: swing.out, crowd: true });
  const font = nameSize(scale);
  const decisions = house.decisions.length;

  /** Walks out of the door: up to it, then, once nobody else swings it, it opens and the founder goes back to the city. */
  const goOut = () => {
    const entry = drawn?.ground.entry;
    if (entry) founder.go(entry, () => setLeaving(founder.get()?.at ?? entry));
  };

  /** A click target over `s` that walks up to it and brings up `opens` in the town's windows. */
  const hit = (s: Spot, key: string, opens: string, title: string, tip: string) => (
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
      // What a tab bar shows stays outlined, wherever the founder walks.
      className={shows(opens) ? 'cursor-pointer stroke-accent' : 'cursor-pointer hover:stroke-accent'}
      onClick={() => {
        founder.go(s.at);
        show(opens, title);
      }}
    >
      <title>{tip}</title>
    </rect>
  );
  const chatAt = (s: Spot, key: string) =>
    hit(s, key, `chat:${s.person.key}`, s.person.name, tipOf(house.people.find((p) => p.key === s.person.key) ?? s.person));

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
                  <Label x={drawn.cast.mine.name[0]} y={drawn.cast.mine.name[1]} font={font} text="You" />
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
              {drawn.cast.desks.map((d) => chatAt(d, `desk-${d.person.key}`))}
              {drawn.cast.papers.map((p) =>
                hit(p, `papers-${p.person.key}`, `tasks|${p.person.key}`, `Tasks · ${p.person.name}`, `${p.person.name}'s tasks`),
              )}
              {drawn.cast.spots.map((s) => chatAt(s, `at-${s.person.key}`))}
              {drawn.front.door && (
                <rect
                  x={drawn.front.door.x}
                  y={drawn.front.door.y}
                  width={drawn.front.door.w}
                  height={drawn.front.door.h}
                  fill="transparent"
                  className="cursor-pointer"
                  onClick={goOut}
                >
                  <title>Go out</title>
                </rect>
              )}
            </svg>
          )}
        </div>
      </div>
    </div>
  );
}
