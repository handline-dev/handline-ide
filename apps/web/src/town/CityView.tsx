// The city drawn as the offices are (street.ts builds it, scene3d.ts and shade.ts draw it), smaller than an office, the view following
// the founder when the city is larger than the pane. Each building has its name on its sign and its counts beside it; a click on one
// walks the founder to its door, which opens, and in, as walking up into a door does; a click on the street walks there. Coming back out
// of a building, the founder steps out of its open door, which shuts behind it.
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { WebGLRenderer } from 'three';
import type { City } from '#src/town/city.ts';
import { FounderDrawing, useFounder, useWalker, type Founder, type Ground } from '#src/town/founder.tsx';
import { groundOf } from '#src/town/ground.ts';
import { SIGN_INK, THROUGH } from '#src/town/palette.ts';
import { boxesOf, type Box } from '#src/town/people.ts';
import { DoorSwing, leavesOf, type Leaves, type Way } from '#src/town/door.tsx';
import { DOOR } from '#src/town/room.ts';
import { drawRoom } from '#src/town/scene3d.ts';
import { shade } from '#src/town/shade.ts';
import { cityRoom, type Door } from '#src/town/street.ts';
import { useSize } from '#src/ui/useSize.ts';
import { nearestFree, type Spot } from '#src/town/walk.ts';

/** What the city shows of a building besides its windows: its name, a line for its tooltip, and its counts. */
export type Sign = { name: string; sub: string; decisions: number; waiting: number; more: number };

type Drawn = { url: string; w: number; h: number; ground: Ground; doors: (Door & { leaves: Leaves | null })[]; boxes: Map<string, Box> };

let renderer: WebGLRenderer | null = null;
let kept: { sig: string; drawn: Drawn } | null = null;

/** The city drawn, again only when its layout, its lit windows or its cars change. */
function useDrawn(city: City, cars: number): Drawn {
  const sig = `${city.w}x${city.h} ${cars} ${city.lots.map((l) => `${l.key}:${l.x}:${l.y}:${l.w}:${l.h}:${l.empty}:${l.seats.join(',')}`).join(' ')}`;
  const lay = () => ({ sig, city, cars });
  const [laid, setLaid] = useState(lay);
  if (laid.sig !== sig) setLaid(lay());
  return useMemo(() => drawCity(laid), [laid]);
}

/** The city for a layout: the kept drawing when it shows the same, else drawn now and kept. */
function drawCity({ sig, city, cars }: { sig: string; city: City; cars: number }): Drawn {
  if (kept?.sig === sig) return kept.drawn;
  const room = cityRoom(city, cars);
  renderer ??= new WebGLRenderer({ antialias: false, alpha: true });
  const frame = drawRoom(renderer, room);
  const pic = shade(frame);
  const canvas = document.createElement('canvas');
  [canvas.width, canvas.height] = [pic.w, pic.h];
  canvas.getContext('2d')?.putImageData(new ImageData(pic.rgba, pic.w, pic.h), 0, 0);
  const ground = groundOf(pic, frame, room, { speed: 2.4, stride: 0.2, grid: { cell: 0.1, body: 0.15, top: 0 } });
  // A doorway's spot is in the building's front, so the founder walks to the nearest it can stand on.
  const doors = room.doors.map((d) => ({
    ...d,
    in: nearestFree(ground.floor, d.in) ?? d.at,
    leaves: leavesOf(room.blocks, `${DOOR}:${d.key}`, 'left'),
  }));
  const drawn = { url: canvas.toDataURL(), w: pic.w, h: pic.h, ground, doors, boxes: boxesOf(pic, frame) };
  kept = { sig, drawn };
  return drawn;
}

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
  left,
  onEnter,
}: {
  city: City;
  cars: number;
  signs: Record<string, Sign>;
  /** The building the founder last came out of: it steps out of that door. None when the city is first shown. */
  left: string | null;
  onEnter: (key: string) => void;
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
  /** Through an open door: in at once from its doorstep, else walking into the doorway first. */
  const walkIn = (f: Founder, d: Door) => {
    const me = f.get();
    if (!me || me.at[1] - d.front < DOORSTEP) onEnter(d.key);
    else f.go(d.in, () => onEnter(d.key));
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
  if (pw === 0) return <div ref={setPane} className="min-h-0 flex-1" />;
  // The offices' size, larger when the whole city fits the pane larger.
  const scale = Math.max(1, Math.min(pw / drawn.w, ph / drawn.h));
  const box = (tag: string) => drawn.boxes.get(tag);
  const doorway = swung && box(`${DOOR}:${swung.key}`);
  return (
    <div ref={setPane} className="relative min-h-0 flex-1 overflow-hidden">
      <Follow founder={founder} ground={walk} scale={scale} size={[drawn.w, drawn.h]} pane={[pw, ph]}>
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
                // Sent somewhere else while it opened: the door shuts as it walks away.
                const me = founder.get();
                const from = swing?.from;
                if (me && from && Math.hypot(me.at[0] - from[0], me.at[1] - from[1]) < 0.05) walkIn(founder, swung);
              }}
            />
          )}
          {drawn.doors.map((d) => {
            const s = signs[d.key];
            const house = box(`house:${d.key}`);
            const sign = box(`sign:${d.key}`);
            if (!s || !house) return null;
            return (
              <g key={d.key} className="cursor-pointer" onClick={() => founder.go(d.at, () => enter(founder, d))}>
                <title>{`${s.name} · ${s.sub}`}</title>
                <rect
                  x={house.x - 1}
                  y={house.y - 1}
                  width={house.w + 2}
                  height={house.h + 2}
                  rx={2}
                  fill="transparent"
                  stroke="transparent"
                  strokeWidth={1.5}
                  className="hover:stroke-accent"
                />
                {sign && (
                  <text
                    x={sign.x + sign.w / 2}
                    y={sign.y + sign.h / 2 + 3}
                    textAnchor="middle"
                    fontSize={8.5}
                    fontWeight={700}
                    fill={SIGN_INK}
                    className="pointer-events-none"
                  >
                    {s.name}
                  </text>
                )}
                {sign && <Badges at={[sign.x + sign.w + 3, sign.y + sign.h / 2]} sign={s} />}
              </g>
            );
          })}
          <FounderDrawing founder={founder} ground={walk} label="You" />
        </svg>
      </Follow>
    </div>
  );
}

/** The city placed in the pane: centred when it fits, else moved so the founder stays in view; only this follows each step. */
function Follow({
  founder,
  ground,
  scale,
  size: [w, h],
  pane: [pw, ph],
  children,
}: {
  founder: Founder;
  ground: Ground;
  scale: number;
  size: readonly [number, number];
  pane: readonly [number, number];
  children: ReactNode;
}) {
  const at = useFounder(founder, (me) => {
    if (!me) return '0,0';
    const [fx, fy] = ground.toPixel(me.at[0], 0, me.at[1]);
    return `${Math.round(fx * scale)},${Math.round(fy * scale)}`;
  });
  const [fx, fy] = at.split(',').map(Number);
  const shift = (size: number, pane: number, f: number) =>
    size <= pane ? (pane - size) / 2 : -Math.min(size - pane, Math.max(0, f - pane / 2));
  return (
    <div className="absolute top-0 left-0" style={{ transform: `translate(${shift(w * scale, pw, fx)}px, ${shift(h * scale, ph, fy)}px)` }}>
      {children}
    </div>
  );
}

/** A building's counts in a row from `at` (the right end of its name sign): decisions, agents waiting, agents past its windows. */
function Badges({ at: [x0, y], sign }: { at: [number, number]; sign: Sign }) {
  const marks = [
    sign.decisions > 0 && { text: String(sign.decisions), fill: 'var(--danger)' },
    sign.waiting > 0 && { text: `!${sign.waiting > 1 ? sign.waiting : ''}`, fill: 'var(--warning)' },
    sign.more > 0 && { text: `+${sign.more}`, fill: 'var(--muted)' },
  ].filter((m) => m !== false);
  let x = x0;
  return marks.map((m) => {
    const w = badgeWidth(m.text);
    x += w + 2;
    return <Badge key={m.fill} x={x - w / 2 - 2} y={y} text={m.text} fill={m.fill} />;
  });
}

const badgeWidth = (text: string) => Math.max(16, text.length * 6.5 + 8);

const Badge = ({ x, y, text, fill }: { x: number; y: number; text: string; fill: string }) => {
  const w = badgeWidth(text);
  return (
    <g className="pointer-events-none">
      <rect x={x - w / 2} y={y - 8} width={w} height={16} rx={8} fill={fill} stroke="var(--bg)" strokeWidth={1.5} />
      <text x={x} y={y + 3.5} textAnchor="middle" fontSize={10} fontWeight={700} fill="var(--accent-fg)">
        {text}
      </text>
    </g>
  );
};
