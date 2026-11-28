import { useEffect, useLayoutEffect, type RefObject } from 'react';

/** Read at no height, so it can shrink as well as grow; a cap is the box's own max-height. */
function fit(el: HTMLTextAreaElement) {
  el.style.height = '0';
  el.style.height = `${el.scrollHeight}px`;
}

/** A text box as tall as its `text`. Laid out narrow first (a tab bar opening from no width), its text and placeholder wrap over many
 *  lines, so it fits again whenever its width changes. */
export function useFitHeight(box: RefObject<HTMLTextAreaElement | null>, text: string) {
  useLayoutEffect(() => {
    if (box.current) fit(box.current);
  }, [box, text]);
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    let wide = el.clientWidth;
    const seen = new ResizeObserver(() => {
      if (el.clientWidth === wide) return;
      wide = el.clientWidth;
      fit(el);
    });
    seen.observe(el);
    return () => seen.disconnect();
  }, [box]);
}
