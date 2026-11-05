// Where each scroll area was left, so going to another tab or agent and coming back finds it where it was. Kept per page load,
// by a name the area gives (`chat:<agent>`, `task:<id>`, ...). A plain area keeps its offset; the chat keeps the line a reader
// was on (useChatScroll), since its lines load after it mounts and an offset alone would land elsewhere.
import { useLayoutEffect, type RefObject } from 'react';

export type KeptLine = { id: string; off: number };
const kept = new Map<string, number | KeptLine | 'end'>();

export const keptScroll = {
  get: (key: string) => kept.get(key),
  set: (key: string, v: number | KeptLine | 'end') => void kept.set(key, v),
};

/** Keeps `ref`'s scroll offset under `key`: put back when it mounts, and again as its content grows in, until the person scrolls. */
export function useKeptScroll(ref: RefObject<HTMLElement | null>, key: string | undefined) {
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || !key) return;
    const at = kept.get(key);
    const want = typeof at === 'number' ? at : 0;
    let settled = want === 0;
    const restore = () => {
      if (settled) return;
      el.scrollTop = want;
      // Done once the content is tall enough to hold the offset.
      if (Math.abs(el.scrollTop - want) < 1) settled = true;
    };
    restore();
    const grow = new ResizeObserver(restore);
    for (const c of el.children) grow.observe(c);
    grow.observe(el);
    const save = () => kept.set(key, el.scrollTop);
    const mine = () => (settled = true);
    el.addEventListener('scroll', save, { passive: true });
    el.addEventListener('wheel', mine, { passive: true });
    el.addEventListener('touchstart', mine, { passive: true });
    el.addEventListener('keydown', mine);
    const stop = setTimeout(() => (settled = true), 3000);
    return () => {
      clearTimeout(stop);
      grow.disconnect();
      el.removeEventListener('scroll', save);
      el.removeEventListener('wheel', mine);
      el.removeEventListener('touchstart', mine);
      el.removeEventListener('keydown', mine);
    };
  }, [ref, key]);
}
