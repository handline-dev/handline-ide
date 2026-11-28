// A workspace's office, shown when its building is clicked: a 3D room drawn as pixel art (room.ts, fit.ts, scene3d.ts, people.ts), drawn
// to fill the pane. The founder walks in it (founder.tsx): a click on the floor walks there, on a person or desk brings up that agent's
// chat (Windows.tsx), on its papers its tasks, on the founder's desk the decisions, on the empty desk hires; Targets.tsx lays these over
// the picture. Marks say who is on hold or stopped (Marks.tsx); a working screen moves (monitors.tsx). The door opens when the
// founder walks into it or it is clicked; Escape never leaves. A person whose place changes walks to the new one (crowd.ts, Cast.tsx).
import { useEffect, useRef, useState, type ReactNode } from 'react';
import type { House } from '#src/town/model.ts';
import { Marks } from '#src/town/Marks.tsx';
import { DESK, Targets } from '#src/town/Targets.tsx';
import { useTownShown } from '#src/town/Windows.tsx';
import { THROUGH } from '#src/town/palette.ts';
import { DoorSwing, type Way } from '#src/town/door.tsx';
import { useWalker, type Founder } from '#src/town/founder.tsx';
import type { Spot as FloorSpot } from '#src/town/walk.ts';
import { useDrawn } from '#src/town/office.ts';
import { Monitors, MotionsOf } from '#src/town/monitors.tsx';
import { useCrowd, usePlaced, useToDesks } from '#src/town/crowd.ts';
import { fadeOf, useFollow } from '#src/town/pan.ts';
import { useCheers } from '#src/town/cheers.ts';
import { Bubbles, JoinOutline, People, READABLE, useSeeThrough, useThroughDoor } from '#src/town/Cast.tsx';

const PIXEL = { imageRendering: 'pixelated' } as const;
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
  const { show } = useTownShown();
  const [pane, [pw, ph]] = useSize();
  const crowd = useCrowd();
  // The same room whatever the space: a smaller pane shows it smaller, but never so small that names shrink; a bigger room follows the
  // founder. One walking to its desk sits before its screen lights.
  const drawn = useDrawn(house, useToDesks(crowd));
  usePlaced(crowd, drawn?.stage ?? null);
  const scale = drawn ? Math.max(READABLE, Math.min(pw / drawn.w, ph / drawn.h)) : 1;
  // The door, pushed open into the room as the founder comes in, shuts behind it; walking down into it pushes it open out, toward the
  // viewer, and then goes back out to the city. One of the people (`crowd`) coming or going holds it open while in the doorway.
  const [swing, setSwing] = useState<{ way: Way; from?: FloorSpot; out?: boolean; crowd?: boolean } | null>(
    cameIn ? { way: 'shut' } : null,
  );
  const founder = useWalker(drawn?.ground ?? null, house.key, ([x, z], [, dz]) => {
    const entry = drawn?.ground.entry;
    if (!swing && entry && dz > 0 && Math.abs(x - entry[0]) < 0.35 && z > entry[1]) setSwing({ way: 'open', from: [x, z], out: true });
  });
  // One whose task is finished celebrates a few seconds.
  const cheers = useCheers();
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
  // How each moves and what its screen shows, from the newest people: the drawing is not made again for it.
  const motions = new Map(house.people.map((p) => [p.key, p.motion ?? null]));
  const decisions = house.decisions.length;

  /** Walks out of the door: up to it, then, once nobody else swings it, it opens and the founder goes back to the city. */
  const goOut = () => {
    const entry = drawn?.ground.entry;
    if (entry) founder.go(entry, () => setLeaving(founder.get()?.at ?? entry));
  };

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
        <Followed
          pane={pane}
          founder={founder}
          room={drawn && { w: drawn.w, h: drawn.h, toPixel: drawn.ground.toPixel }}
          size={[pw, ph]}
          scale={scale}
        >
          {drawn && pw > 0 && (
            <>
              <MotionsOf value={motions}>
                <svg
                  width={drawn.w * scale}
                  height={drawn.h * scale}
                  viewBox={`0 0 ${drawn.w} ${drawn.h}`}
                  className="block"
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
                  <Monitors screens={drawn.screens} />
                  {drawn.cast.join && <JoinOutline join={drawn.cast.join} />}
                  <People drawn={drawn} founder={founder} crowd={crowd} scale={scale} cheers={cheers} />
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
                  <Marks drawn={drawn} crowd={crowd} people={house.people} scale={scale} cheers={cheers} />
                </svg>
              </MotionsOf>
              <Targets drawn={drawn} scale={scale} house={house} founder={founder} onOut={goOut} />
            </>
          )}
        </Followed>
      </div>
    </div>
  );
}

type Room = { w: number; h: number; toPixel: (x: number, y: number, z: number) => [number, number] };

/** The pane, with the room placed where the founder's walk has it and the cut edges faded; only this draws again at each step. */
function Followed(props: {
  pane: React.RefObject<HTMLDivElement | null>;
  founder: Founder;
  room: Room | null;
  size: [number, number];
  scale: number;
  children: ReactNode;
}) {
  const { pane, founder, room, size, scale } = props;
  const view = useFollow(founder, room?.toPixel ?? null, room ? [room.w, room.h] : [0, 0], size, scale);
  return (
    <div
      ref={pane}
      className="relative min-h-0 min-w-0 flex-1 overflow-hidden"
      style={fadeOf(view.more)}
      // Focus going to something out of view scrolls even a pane that hides its overflow; only the walk moves the view.
      onScroll={(e) => {
        e.currentTarget.scrollLeft = 0;
        e.currentTarget.scrollTop = 0;
      }}
    >
      {room && (
        <div
          className="absolute top-0 left-0"
          style={{ width: room.w * scale, height: room.h * scale, transform: `translate(${view.at[0]}px, ${view.at[1]}px)` }}
        >
          {props.children}
        </div>
      )}
    </div>
  );
}
