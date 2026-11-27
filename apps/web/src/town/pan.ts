// An office bigger than its pane scrolls; each edge with more of the room beyond it fades out, so the cut reads as "there is more".
import { useLayoutEffect, useState, type RefObject } from 'react';

/** Which edges of a scrolled pane have more beyond them: left, right, top, bottom. */
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

/** The pane's edges with more beyond them, kept as it scrolls or the room or pane changes size (`size` changing says so). */
export function useMore(pane: RefObject<HTMLElement | null>, size: unknown): More {
  const [more, setMore] = useState<More>([false, false, false, false]);
  useLayoutEffect(() => {
    const el = pane.current;
    if (!el) return;
    const look = () => {
      const next = moreOf(el.scrollLeft, el.scrollTop, el.clientWidth, el.clientHeight, el.scrollWidth, el.scrollHeight);
      setMore((was) => (was.every((m, k) => m === next[k]) ? was : next));
    };
    look();
    el.addEventListener('scroll', look, { passive: true });
    return () => el.removeEventListener('scroll', look);
  }, [pane, size]);
  return more;
}
