// A working agent's screen moves: its lines of text scroll up. Each lit screen the picture shows (by its tag, room.ts screenOf) becomes a
// strip of frames, one under the next, painted only on that screen's own pixels, so whoever sits in front of it still covers it.
import type { Picture } from '#src/town/shade.ts';
import type { Frame } from '#src/town/scene3d.ts';

/** One moving screen: where its frames go on the picture, and the strip of `frames` frames, each `h` tall; `id` is its object's id in
 *  the picture and `pixel` the index of one of its pixels. */
export type Screen = { x: number; y: number; w: number; h: number; url: string; frames: number; id: number; pixel: number };

/** Lines of text before the strip repeats; it scrolls a pixel a frame, a line being a row of text over a blank row. */
const LINES = 6;
const FRAMES = LINES * 2;
const DIM = 0.6;

/** A line's length and its word breaks, the same for the same line. */
const ink = (line: number, x: number, w: number) => {
  const n = ((line * 7919) % 13) / 13;
  const len = Math.max(3, Math.round(w * (0.35 + 0.55 * n)));
  return x >= 1 && x < len && (x + line * 3) % 6 !== 5;
};

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
  return [...spots.values()].map((spot) => stripOf(pic, spot));
}

function stripOf(pic: Picture, spot: number[]): Screen {
  const xs = spot.map((i) => i % pic.w);
  const ys = spot.map((i) => Math.floor(i / pic.w));
  const [x0, y0] = [Math.min(...xs), Math.min(...ys)];
  const [w, h] = [Math.max(...xs) - x0 + 1, Math.max(...ys) - y0 + 1];
  // The screen's lightest pixel is its glow; text is that, dimmed.
  let bg = spot[0];
  const sum = (i: number) => pic.rgba[i * 4] + pic.rgba[i * 4 + 1] + pic.rgba[i * 4 + 2];
  for (const i of spot) if (sum(i) > sum(bg)) bg = i;
  const glow = [...pic.rgba.subarray(bg * 4, bg * 4 + 3)];
  const img = new ImageData(w, h * FRAMES);
  for (let f = 0; f < FRAMES; f++)
    for (const i of spot) {
      const [x, y] = [(i % pic.w) - x0, Math.floor(i / pic.w) - y0];
      const row = y + f;
      const dark = row % 2 === 0 && ink(Math.floor(row / 2) % LINES, x, w);
      img.data.set([...glow.map((c) => (dark ? c * DIM : c)), 255], ((f * h + y) * w + x) * 4);
    }
  const canvas = document.createElement('canvas');
  [canvas.width, canvas.height] = [w, h * FRAMES];
  canvas.getContext('2d')?.putImageData(img, 0, 0);
  return { x: x0, y: y0, w, h, url: canvas.toDataURL(), frames: FRAMES, id: pic.id[spot[0]], pixel: spot[0] };
}

const FRAME_MS = 180;

/** The moving screens over the room's picture, each its strip seen through a window one frame tall, stepping a frame at a time; still
 *  on its first frame when the viewer asks for less motion. */
export function Monitors({ screens }: { screens: Screen[] }) {
  const still = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  return (
    <g className="pointer-events-none">
      {screens.map((s) => (
        <svg key={`${s.x},${s.y}`} x={s.x} y={s.y} width={s.w} height={s.h} viewBox={`0 0 ${s.w} ${s.h}`} overflow="hidden">
          <image href={s.url} width={s.w} height={s.h * s.frames} style={{ imageRendering: 'pixelated' }}>
            {!still && (
              <animate
                attributeName="y"
                values={Array.from({ length: s.frames }, (_, f) => -f * s.h).join(';')}
                dur={`${s.frames * FRAME_MS}ms`}
                calcMode="discrete"
                repeatCount="indefinite"
              />
            )}
          </image>
        </svg>
      ))}
    </g>
  );
}
