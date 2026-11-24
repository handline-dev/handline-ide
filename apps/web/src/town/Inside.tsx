// A workspace's office, shown in place of the city when its building is clicked: a 3D room drawn as pixel art (room.ts lays it out,
// scene3d.ts and shade.ts draw it, people.ts seats the people), at the largest whole-pixel scale that fits the pane. A click on a person
// or a desk opens that agent's chat.
import { useEffect, useMemo, useRef, useState } from 'react';
import { WebGLRenderer } from 'three';
import { WORDS } from '#src/town/panel.tsx';
import type { House, Person } from '#src/town/model.ts';
import { castInto, OFFICE_SHEET, type Cast, type Spot } from '#src/town/people.ts';
import { roomOf } from '#src/town/room.ts';
import { drawRoom } from '#src/town/scene3d.ts';
import { shade } from '#src/town/shade.ts';
import { NAME_HALO, NAME_INK } from '#src/town/palette.ts';

const PIXEL = { imageRendering: 'pixelated' } as const;

let renderer: WebGLRenderer | null = null;

/** The people sheet's pixels, once loaded. */
function useSheet(): ImageData | null {
  const [sheet, setSheet] = useState<ImageData | null>(null);
  useEffect(() => {
    const img = new Image();
    img.onload = () => {
      const c = document.createElement('canvas');
      [c.width, c.height] = [img.width, img.height];
      const ctx = c.getContext('2d');
      ctx?.drawImage(img, 0, 0);
      if (ctx) setSheet(ctx.getImageData(0, 0, img.width, img.height));
    };
    img.src = OFFICE_SHEET;
  }, []);
  return sheet;
}

type Drawn = { url: string; w: number; h: number; cast: Cast };

/** The room drawn with its people, as an image URL and where each person and desk is; redrawn when the house changes. */
function useDrawn(house: House, sheet: ImageData | null): Drawn | null {
  const room = useMemo(() => roomOf(house), [house]);
  const [drawn, setDrawn] = useState<Drawn | null>(null);
  useEffect(() => {
    if (!sheet) return;
    let live = true;
    renderer ??= new WebGLRenderer({ antialias: false, alpha: true });
    void drawRoom(renderer, room).then((frame) => {
      if (!live) return;
      const pic = shade(frame);
      const cast = castInto(pic, frame, room, sheet);
      const canvas = document.createElement('canvas');
      [canvas.width, canvas.height] = [pic.w, pic.h];
      canvas.getContext('2d')?.putImageData(new ImageData(pic.rgba, pic.w, pic.h), 0, 0);
      setDrawn({ url: canvas.toDataURL(), w: pic.w, h: pic.h, cast });
    });
    return () => {
      live = false;
    };
  }, [room, sheet]);
  return drawn;
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
  onTalk,
  onMail,
}: {
  house: House;
  onBack: () => void;
  onTalk: (p: Person) => void;
  onMail: () => void;
}) {
  const drawn = useDrawn(house, useSheet());
  const [pane, [pw, ph]] = useSize();
  const scale = drawn ? Math.max(1, Math.floor(Math.min(pw / drawn.w, ph / drawn.h))) : 1;
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
      onClick={() => onTalk(s.person)}
    >
      <title>{tipOf(s.person)}</title>
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
      <div ref={pane} className="flex min-h-0 flex-1 items-center justify-center overflow-hidden p-4">
        {drawn && pw > 0 && (
          <svg
            width={drawn.w * scale}
            height={drawn.h * scale}
            viewBox={`0 0 ${drawn.w} ${drawn.h}`}
            className="shrink-0"
            role="img"
            aria-label={`${house.name}'s office`}
          >
            <image href={drawn.url} width={drawn.w} height={drawn.h} style={PIXEL} />
            {drawn.cast.names.map((n) => (
              <text
                key={n.person.key}
                x={n.x}
                y={n.y + font}
                textAnchor="middle"
                fontSize={font}
                fontWeight={600}
                fill={NAME_INK}
                stroke={NAME_HALO}
                strokeWidth={2.5 / scale}
                paintOrder="stroke"
                className="pointer-events-none"
              >
                {n.person.name}
              </text>
            ))}
            {drawn.cast.desks.map((d) => hit(d, `desk-${d.person.key}`))}
            {drawn.cast.spots.map((s) => hit(s, `at-${s.person.key}`))}
          </svg>
        )}
      </div>
    </div>
  );
}
