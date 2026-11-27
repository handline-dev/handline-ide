// The city drawn as the offices are (street.ts builds it, scene3d.ts and shade.ts draw it), smaller than an office. The wheel, the corner
// buttons and M zoom it between the whole town in the pane and the close-up, where the view follows the founder (camera.tsx). Each
// building has its name on its sign and its counts beside it (Signs.tsx); a click on one walks the founder to its door, which opens, and
// in, as walking up into a door does; a click on the street walks there. A double click, any click with the whole town in view, and a
// pick from Find (Find.tsx) stand the founder at that door at once. Coming back out of a building, the founder steps out of its open
// door, which shuts behind it. The building site is gone to as a building is; arriving there opens the form that adds a workspace.
import { useEffect, useMemo, useRef, useState } from 'react';
import { newWorkspace } from '#src/NewWorkspace.tsx';
import { Placed, useCamera, useCityKeys, useWheelZoom } from '#src/town/camera.tsx';
import { HQ, SITE, type City } from '#src/town/city.ts';
import { CityControls } from '#src/town/CityControls.tsx';
import { useDrawn } from '#src/town/cityPicture.ts';
import { Find } from '#src/town/Find.tsx';
import type { Found } from '#src/town/find.ts';
import { FounderDrawing, useFounder, useWalker, type Founder } from '#src/town/founder.tsx';
import type { House } from '#src/town/model.ts';
import { THROUGH } from '#src/town/palette.ts';
import { DoorSwing, type Way } from '#src/town/door.tsx';
import { DOOR } from '#src/town/room.ts';
import { Signs, YouTag, type Sign } from '#src/town/Signs.tsx';
import type { Door } from '#src/town/street.ts';
import { useSize } from '#src/ui/useSize.ts';
import type { Spot } from '#src/town/walk.ts';

export type { Sign };

/** The building site's sign: what a click on it does. */
const SITE_SIGN: Sign = { name: '+ New team', short: '+', sub: '', decisions: 0, waiting: 0, stopped: 0, more: 0, action: true };
/** Narrower than this, the pane shows Find as an icon. */
const NARROW = 520;
/** How long after the click that asks for it the form that adds a team opens. */
const FORM_MS = 250;

/** How close below a building's front the founder must be, in metres, to be at its door: walking up there takes it in, and its door
 *  stays open while it is there. */
const AT_DOOR = 0.7;
/** How close the founder stands to a front on its doorstep, where held keys stop it: it goes in from there. */
const DOORSTEP = 0.3;
const atDoor = (d: Door, [x, z]: Spot) => x > d.x0 && x < d.x1 && z > d.front && z - d.front < AT_DOOR;
/** A door swinging; `from` is where the founder stood when it began to open. */
type Swinging = { key: string; way: Way; from?: Spot };

export function CityView({
  city,
  cars,
  signs,
  houses,
  left,
  onEnter,
}: {
  city: City;
  cars: number;
  signs: Record<string, Sign>;
  /** The teams and their people, for Find. */
  houses: readonly House[];
  /** The building the founder last came out of: it steps out of that door. None when the city is first shown. */
  left: string | null;
  /** The founder goes into a building; `person` is who was looked for there, when it was found by a person. */
  onEnter: (key: string, person?: string) => void;
}) {
  const drawn = useDrawn(city, cars);
  const [pane, setPane] = useState<HTMLDivElement | null>(null);
  const [pw, ph] = useSize(pane, (el) => [el.clientWidth, el.clientHeight] as const, [0, 0] as const);
  const walk = useMemo(() => {
    const door = drawn.doors.find((d) => d.key === left);
    return door ? { ...drawn.ground, entry: door.in } : drawn.ground;
  }, [drawn, left]);
  // The door swinging, if any: one the founder came out of starts open. `live` is the latest, for walks that end after a render.
  const [swing, setSwing] = useState<Swinging | null>(() => (left ? { key: left, way: 'held' } : null));
  const live = useRef(swing);
  useEffect(() => {
    live.current = swing;
  });
  const swingTo = (next: Swinging | null) => {
    live.current = next;
    setSwing(next);
  };
  // Who Find was asked for, and in which building: told when the founder goes in there, forgotten when it is sent elsewhere.
  const sought = useRef<{ house: string; person?: string } | null>(null);
  const goIn = (key: string) => onEnter(key, sought.current?.house === key ? sought.current.person : undefined);
  /** Through an open door: in at once from its doorstep, else walking into the doorway first. */
  const walkIn = (f: Founder, d: Door) => {
    const me = f.get();
    if (!me || me.at[1] - d.front < DOORSTEP) goIn(d.key);
    else f.go(d.in, () => goIn(d.key));
  };
  /** At a door: it opens, and the founder goes in once it is open. Another door swinging snaps shut. */
  const enter = (f: Founder, d: Door) => {
    const s = live.current;
    if (s?.key === d.key && s.way === 'held') walkIn(f, d);
    else if (s?.key !== d.key || s.way === 'shut') swingTo({ key: d.key, way: 'open', from: f.get()?.at });
  };
  const founder = useWalker(walk, 'city', (at, [, dz], f) => {
    const door = dz < 0 && drawn.doors.find((d) => atDoor(d, at));
    if (door) enter(f, door);
  });
  const { camera, level, scale, can, whole } = useCamera(founder, walk, [drawn.w, drawn.h], [pw, ph]);
  useWheelZoom(pane, camera);
  const field = useRef<HTMLInputElement>(null);
  useCityKeys({ find: () => field.current?.focus(), toggle: () => can && camera.toggle() });
  const swung = drawn.doors.find((d) => d.key === swing?.key);
  const near = useFounder(founder, (me) => !me || !swung || atDoor(swung, me.at));
  // Out of the door the founder came out of; it shuts once the founder is past the doorstep.
  const out = useRef(left);
  useEffect(() => {
    const d = drawn.doors.find((d) => d.key === out.current);
    if (!d) return;
    out.current = null;
    founder.go(d.out);
  }, [walk, drawn, founder]);
  const way = swing && (swing.way === 'held' && !near ? 'shut' : swing.way);
  // The form opens a moment after the click that asks for it, so the second click of a double click does not land on it and close it.
  const opening = useRef(0);
  useEffect(() => () => clearTimeout(opening.current), []);
  const addTeam = () => {
    clearTimeout(opening.current);
    opening.current = window.setTimeout(newWorkspace, FORM_MS);
  };
  if (pw === 0) return <div ref={setPane} className="min-h-0 flex-1" />;
  /** What arriving before a building does: its door opens and the founder goes in; at the building site, the form that adds a team. */
  const arrive = (d: Door) => (d.key === SITE ? addTeam() : enter(founder, d));
  /** To a building with no walk: the founder stands at its door. */
  const rush = (d: Door, person?: string) => {
    sought.current = { house: d.key, person };
    founder.put(d.at);
    arrive(d);
  };
  const pick = (d: Door, twice: boolean) => {
    if (twice || whole) return rush(d);
    sought.current = null;
    founder.go(d.at, () => arrive(d));
  };
  const places = drawn.site ? [...drawn.doors, drawn.site] : drawn.doors;
  const find = (found: Found) => {
    const d = places.find((d) => d.key === found.house);
    if (d) rush(d, found.person);
  };
  const doorway = swung && drawn.boxes.get(`${DOOR}:${swung.key}`);
  return (
    <div ref={setPane} className="relative min-h-0 flex-1 overflow-hidden">
      <Placed camera={camera}>
        <svg
          width={drawn.w * scale}
          height={drawn.h * scale}
          viewBox={`0 0 ${drawn.w} ${drawn.h}`}
          role="img"
          aria-label="Your team as a city"
        >
          <image
            href={drawn.url}
            width={drawn.w}
            height={drawn.h}
            style={{ imageRendering: 'pixelated' }}
            onClick={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              sought.current = null;
              founder.go(walk.toFloor((e.clientX - r.left) / scale, (e.clientY - r.top) / scale));
            }}
          />
          {swung?.leaves && doorway && way && (
            <DoorSwing
              key={swung.key}
              pic={drawn.ground.pic}
              door={{ box: doorway, through: THROUGH, leaves: swung.leaves }}
              toPixel={drawn.ground.toPixel}
              way={way}
              onDone={() => {
                if (way === 'shut') return swingTo(null);
                if (way !== 'open') return;
                swingTo({ key: swung.key, way: 'held' });
                // Sent somewhere else while it opened: the door shuts as it walks away, and nobody is looked for any more.
                const me = founder.get();
                const from = swing?.from;
                if (me && from && Math.hypot(me.at[0] - from[0], me.at[1] - from[1]) < 0.05) walkIn(founder, swung);
                else sought.current = null;
              }}
            />
          )}
          <Signs doors={places} signs={{ ...signs, [SITE]: SITE_SIGN }} boxes={drawn.boxes} scale={scale} onPick={pick} />
          <FounderDrawing founder={founder} ground={walk} label={scale < 1 ? undefined : 'You'} />
          {scale < 1 && <YouTag founder={founder} ground={walk} scale={scale} />}
        </svg>
      </Placed>
      <Find
        field={field}
        houses={[{ key: HQ, name: signs[HQ]?.name ?? 'Your HQ', people: [] }, ...houses]}
        compact={pw < NARROW}
        onPick={find}
      />
      {can && <CityControls level={level} onStep={camera.step} onToggle={camera.toggle} />}
    </div>
  );
}
