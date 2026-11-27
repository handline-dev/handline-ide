// A working agent's screen moves by what it does (motion.ts): reading, its lines of text scroll up; writing, a line types out on a white
// page with a cursor at its end; running something, lines run up a dark terminal. Each lit screen the picture shows (by its tag, room.ts
// screenOf) becomes a strip of frames per kind, one frame under the next, painted only on that screen's own pixels, so whoever sits in
// front of it still covers it.
import { createContext, use } from 'react';
import type { Picture } from '#src/town/shade.ts';
import type { Frame } from '#src/town/scene3d.ts';
import type { Motion } from '#src/town/motion.ts';

/** What a screen shows while its agent works. */
export type Kind = 'scroll' | 'type' | 'terminal';
/** One moving screen: where its frames go on the picture, and a strip per kind of `frames` frames, each `h` tall; `key` is its agent's,
 *  `id` its object's id in the picture and `pixel` the index of one of its pixels. */
export type Screen = {
  key: string;
  x: number;
  y: number;
  w: number;
  h: number;
  strips: Record<Kind, { url: string; frames: number }>;
  id: number;
  pixel: number;
};

/** How each agent moves, by key, for the screens (Inside.tsx gives the newest). */
export const MotionsOf = createContext<ReadonlyMap<string, Motion | null>>(new Map());

/** The screen of one moving so. */
export const screenKind = (motion: Motion | null | undefined): Kind =>
  motion === 'writing' ? 'type' : motion === 'running' ? 'terminal' : 'scroll';

/** Lines of text before a strip repeats, a line being a row of text over a blank row. */
const LINES = 6;
const DIM = 0.6;
/** A terminal's ground, of the screen's glow. */
const DARK = 0.22;
/** How far a page being written is from the glow toward white, so it reads apart from one being read. */
const PAGE = 0.75;
const FRAME_MS: Record<Kind, number> = { scroll: 180, type: 150, terminal: 130 };

/** A line's length, the same for the same line. */
const lengthOf = (line: number, w: number) => Math.max(3, Math.round(w * (0.35 + 0.55 * (((line * 7919) % 13) / 13))));
/** Whether a line has ink at x: its word breaks. */
const ink = (line: number, x: number, w: number) => x >= 1 && x < lengthOf(line, w) && (x + line * 3) % 6 !== 5;

/** For a frame of a kind, whether (x, y) of a screen w by h is ink, and whether it is the cursor. */
export function painter(
  kind: Kind,
  w: number,
  h: number,
): { frames: number; at: (f: number, x: number, y: number) => 'ink' | 'cursor' | null } {
  if (kind !== 'type') {
    const frames = LINES * 2;
    return { frames, at: (f, x, y) => ((y + f) % 2 === 0 && ink(Math.floor((y + f) / 2) % LINES, x, w) ? 'ink' : null) };
  }
  // One letter a frame along each line, the cursor after the last; once the screen is full its lines move up one.
  const rows = Math.max(1, Math.floor((h + 1) / 2));
  const starts: number[] = [];
  let n = 0;
  for (let l = 0; l < LINES; l++) {
    starts.push(n);
    n += lengthOf(l, w);
  }
  return {
    frames: n,
    at: (f, x, y) => {
      const line = starts.filter((s) => s <= f).length - 1;
      const top = Math.max(0, line - rows + 1);
      if (y % 2 !== 0) return null;
      const l = top + y / 2;
      if (l > line) return null;
      const typed = l < line ? lengthOf(l, w) : f - starts[l] + 1;
      if (l === line && x === typed) return 'cursor';
      return x < typed && ink(l, x, w) ? 'ink' : null;
    },
  };
}

/** The moving screens on a picture: every tag starting `screen:`. */
export function screensOf(pic: Picture, frame: Frame): Screen[] {
  const spots = new Map<string, number[]>();
  for (let i = 0; i < pic.id.length; i++) {
    const tag = frame.meta[pic.id[i]]?.tag;
    if (!tag?.startsWith('screen:')) continue;
    const list = spots.get(tag);
    if (list) list.push(i);
    else spots.set(tag, [i]);
  }
  return [...spots].map(([tag, spot]) => stripsOf(pic, tag.slice('screen:'.length), spot));
}

function stripsOf(pic: Picture, key: string, spot: number[]): Screen {
  const xs = spot.map((i) => i % pic.w);
  const ys = spot.map((i) => Math.floor(i / pic.w));
  const [x0, y0] = [Math.min(...xs), Math.min(...ys)];
  const [w, h] = [Math.max(...xs) - x0 + 1, Math.max(...ys) - y0 + 1];
  // The screen's lightest pixel is its glow; text is that, dimmed.
  let bg = spot[0];
  const sum = (i: number) => pic.rgba[i * 4] + pic.rgba[i * 4 + 1] + pic.rgba[i * 4 + 2];
  for (const i of spot) if (sum(i) > sum(bg)) bg = i;
  const glow = [...pic.rgba.subarray(bg * 4, bg * 4 + 3)];
  const strip = (kind: Kind) => {
    const { frames, at } = painter(kind, w, h);
    const [ground, text] = kind === 'terminal' ? [DARK, 1] : [1, DIM];
    const img = new ImageData(w, h * frames);
    for (let f = 0; f < frames; f++)
      for (const i of spot) {
        const [x, y] = [(i % pic.w) - x0, Math.floor(i / pic.w) - y0];
        const p = at(f, x, y);
        const shade = p === 'cursor' ? DIM * DIM : p ? text : ground;
        // A page being written is white where it is blank.
        const page = kind === 'type' && !p ? PAGE : 0;
        img.data.set([...glow.map((c) => c * shade + (255 - c * shade) * page), 255], ((f * h + y) * w + x) * 4);
      }
    const canvas = document.createElement('canvas');
    [canvas.width, canvas.height] = [w, h * frames];
    canvas.getContext('2d')?.putImageData(img, 0, 0);
    return { url: canvas.toDataURL(), frames };
  };
  const strips = { scroll: strip('scroll'), type: strip('type'), terminal: strip('terminal') };
  return { key, x: x0, y: y0, w, h, strips, id: pic.id[spot[0]], pixel: spot[0] };
}

/** The moving screens over the room's picture, each the strip of what its agent does seen through a window one frame tall, stepping a
 *  frame at a time; still on its first frame when the viewer asks for less motion. */
export function Monitors({ screens }: { screens: Screen[] }) {
  const motions = use(MotionsOf);
  const still = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  return (
    <g className="pointer-events-none">
      {screens.map((s) => {
        const kind = screenKind(motions.get(s.key));
        const { url, frames } = s.strips[kind];
        return (
          <svg key={`${s.x},${s.y}`} x={s.x} y={s.y} width={s.w} height={s.h} viewBox={`0 0 ${s.w} ${s.h}`} overflow="hidden">
            <image key={kind} href={url} width={s.w} height={s.h * frames} style={{ imageRendering: 'pixelated' }}>
              {!still && (
                <animate
                  attributeName="y"
                  values={Array.from({ length: frames }, (_, f) => -f * s.h).join(';')}
                  dur={`${frames * FRAME_MS[kind]}ms`}
                  calcMode="discrete"
                  repeatCount="indefinite"
                />
              )}
            </image>
          </svg>
        );
      })}
    </g>
  );
}
