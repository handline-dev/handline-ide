// A workspace's office, shown in place of the city when its building is clicked: a 3D room drawn as pixel art (room.ts lays it out,
// scene3d.ts and shade.ts draw it, people.ts seats the people), its benches as wide as fill the pane best and the picture filling it.
// The founder walks in it (founder.tsx): a click on the floor walks there, one on a person or a desk walks up to it. Standing beside an
// agent floats its chat over the room (floater.tsx), beside the founder's own desk its decisions; walking away puts it away.
import { useEffect, useMemo, useRef, useState } from 'react';
import { WebGLRenderer } from 'three';
import { Decisions, WORDS } from '#src/town/panel.tsx';
import type { Decision, House, Person } from '#src/town/model.ts';
import { Chat } from '#src/chat/Chat.tsx';
import { Floater } from '#src/town/floater.tsx';
import { BUBBLE, castOf, PEOPLE_SHEET, SHEET, type Cast, type Figure, type Spot } from '#src/town/people.ts';
import { COLS, roomOf } from '#src/town/room.ts';
import { viewOf } from '#src/town/view.ts';
import { drawRoom } from '#src/town/scene3d.ts';
import { shade } from '#src/town/shade.ts';
import { NAME_HALO, NAME_INK } from '#src/town/palette.ts';
import { FounderDrawing, useWalker, type Ground } from '#src/town/founder.tsx';
import { floorOf, type Spot as FloorSpot } from '#src/town/walk.ts';
import { Vector3 } from 'three';
import type { Frame } from '#src/town/scene3d.ts';
import type { Room } from '#src/town/room.ts';
import type { Picture } from '#src/town/shade.ts';

const PIXEL = { imageRendering: 'pixelated' } as const;

let renderer: WebGLRenderer | null = null;

type Drawn = { url: string; w: number; h: number; cast: Cast; ground: Ground };

/** What a drawing shows: the bench width and each person's name and state, so a poll that changes nothing draws nothing. */
const sigOf = (house: House, cols: number) =>
  [house.key, cols, house.decisions.length, ...house.people.map((p) => `${p.key}:${p.name}:${p.activity}:${p.queue}`)].join(' ');
/** The last drawing of each house, so the office is there at once when its tab comes back. */
const drawings = new Map<string, { sig: string; drawn: Drawn }>();

/** The room drawn with its people, as an image URL and where each person and desk is; redrawn only when what it shows changes. */
/** `cols` is null until the pane has a size. */
function useDrawn(house: House, cols: number | null): Drawn | null {
  const sig = cols === null ? null : sigOf(house, cols);
  const [drawn, setDrawn] = useState<Drawn | null>(() => drawings.get(house.key)?.drawn ?? null);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- the room is laid out again only when what it shows changes, not on each poll
  const room = useMemo(() => (cols === null ? null : roomOf(house, cols)), [sig]);
  useEffect(() => {
    if (!room || drawings.get(house.key)?.sig === sig) return; // the kept drawing is already shown
    let live = true;
    renderer ??= new WebGLRenderer({ antialias: false, alpha: true });
    void drawRoom(renderer, room).then((frame) => {
      if (!live) return;
      const pic = shade(frame);
      const cast = castOf(pic, frame, room);
      const canvas = document.createElement('canvas');
      [canvas.width, canvas.height] = [pic.w, pic.h];
      canvas.getContext('2d')?.putImageData(new ImageData(pic.rgba, pic.w, pic.h), 0, 0);
      const next = { url: canvas.toDataURL(), w: pic.w, h: pic.h, cast, ground: groundOf(pic, frame, room) };
      drawings.set(house.key, { sig: sig ?? '', drawn: next });
      setDrawn(next);
    });
    return () => {
      live = false;
    };
  }, [room]); // eslint-disable-line react-hooks/exhaustive-deps -- room changes exactly when sig does
  return drawn;
}

/** What the founder walks on: the room's footprints as a grid, and the camera's sums both ways. */
function groundOf(pic: Picture, frame: Frame, room: Room): Ground {
  const toCamera = frame.camera.matrixWorldInverse;
  const toFloor = (px: number, py: number): FloorSpot => {
    // Newton's method on the floor's projection, from the room's middle: it is smooth, so a few steps land within a millimetre.
    let [x, z] = [room.w / 2, room.d / 2];
    for (let k = 0; k < 8; k++) {
      const [ax, ay] = frame.toPixel(x, 0, z);
      const [bx, by] = frame.toPixel(x + 0.01, 0, z);
      const [cx, cy] = frame.toPixel(x, 0, z + 0.01);
      const [j11, j21, j12, j22] = [(bx - ax) / 0.01, (by - ay) / 0.01, (cx - ax) / 0.01, (cy - ay) / 0.01];
      const det = j11 * j22 - j12 * j21;
      const [ex, ey] = [px - ax, py - ay];
      x += (j22 * ex - j12 * ey) / det;
      z += (-j21 * ex + j11 * ey) / det;
    }
    return [x, z];
  };
  return {
    pic,
    kinds: frame.meta.map((m) => m.kind),
    floor: floorOf(room.w, room.d, frame.solid),
    entry: room.entry,
    toPixel: frame.toPixel,
    toFloor,
    depthOf: (x, y, z) => -new Vector3(x, y, z).applyMatrix4(toCamera).z,
  };
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
  onBack,
  onOpen,
  onMail,
}: {
  house: House;
  onBack: () => void;
  onOpen: (d: Decision) => void;
  onMail: () => void;
}) {
  const [pane, [pw, ph]] = useSize();
  const cols = useMemo(() => (pw > 0 && ph > 0 ? colsFor(house, pw, ph) : null), [house, pw, ph]);
  const drawn = useDrawn(house, cols);
  const scale = drawn ? Math.min(pw / drawn.w, ph / drawn.h) : 1;
  const { me, go } = useWalker(drawn?.ground ?? null, house.key);
  const near = me && drawn ? nearOf(drawn.cast, me.at) : null;
  const nearPerson = near && near !== 'mine' ? (house.people.find((p) => p.key === near) ?? null) : null;
  const font = 11 / scale; // names stay 11px on screen whatever the scale
  const decisions = house.decisions.length;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onBack();
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
      onClick={() => go(s.at)}
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
      <div className="flex min-h-0 flex-1 p-4">
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
                  go(drawn.ground.toFloor((e.clientX - r.left) / scale, (e.clientY - r.top) / scale));
                }}
              />
              {drawn.cast.figures
                .filter((f) => !me || f.z <= me.at[1])
                .map((f) => (
                  <Drawing key={f.person.key} figure={f} />
                ))}
              {me && <FounderDrawing me={me} ground={drawn.ground} />}
              {drawn.cast.figures
                .filter((f) => me && f.z > me.at[1])
                .map((f) => (
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
              {drawn.cast.names.map((n) => (
                <Label key={n.person.key} x={n.x} y={n.y} font={font} scale={scale} text={n.person.name} />
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
                    onClick={() => drawn.cast.mine && go(drawn.cast.mine.at)}
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
