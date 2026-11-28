// An office bigger than its pane keeps the founder in the middle of the view as it walks, never showing past the room's edges; each edge
// with more of the room beyond it fades out, so the cut reads as "there is more".
import { useSyncExternalStore } from 'react';
import type { Founder } from '#src/town/founder.tsx';
import { inPane, type Pair } from '#src/town/zoom.ts';

/** Which edges of the view have more of the room beyond them: left, right, top, bottom. */
export type More = [boolean, boolean, boolean, boolean];

/** How far the fade reaches into the pane, in pixels. */
const FADE = 40;

/** The more of a pane scrolled to (left, top), showing w by h of a sw by sh room. */
export const moreOf = (left: number, top: number, w: number, h: number, sw: number, sh: number): More => [
  left > 0.5,
  left + w < sw - 0.5,
  top > 0.5,
  top + h < sh - 0.5,
];

/** The mask that fades each edge with more beyond it; none when nothing is cut. */
export function fadeOf(more: More): React.CSSProperties {
  if (!more.some(Boolean)) return {};
  const sides = ['to right', 'to left', 'to bottom', 'to top'] as const;
  const mask = sides.map((to, k) => `linear-gradient(${to}, transparent, black ${more[k] ? FADE : 0}px)`).join(', ');
  return { maskImage: mask, maskComposite: 'intersect' };
}

/** The room's top left in the pane, in whole pixels: the founder's `spot` (picture pixels) in the middle where the room allows, the room
 *  in the middle where it fits, its top left where the founder is not there yet. */
export function followOf(spot: Pair | null, size: Pair, pane: Pair, scale: number): Pair {
  const o: Pair = spot ? [pane[0] / 2 - spot[0] * scale, pane[1] / 2 - spot[1] * scale] : [0, 0];
  const [x, y] = inPane(o, size, pane, scale);
  return [Math.round(x), Math.round(y)];
}

/** Where the room sits in the pane as the founder walks, and which edges have more beyond them. */
export function useFollow(
  founder: Founder,
  toPixel: ((x: number, y: number, z: number) => Pair) | null,
  size: Pair,
  pane: Pair,
  scale: number,
) {
  const at = useSyncExternalStore(founder.subscribe, () => {
    const me = founder.get();
    return followOf(me && toPixel ? toPixel(me.at[0], 0, me.at[1]) : null, size, pane, scale).join(',');
  });
  const [x, y] = at.split(',').map(Number);
  return { at: [x, y] as Pair, more: moreOf(-x, -y, pane[0], pane[1], size[0] * scale, size[1] * scale) };
}
