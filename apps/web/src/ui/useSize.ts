import { type RefObject, useLayoutEffect, useRef, useState } from 'react';

/** `read(el)` now and again on each resize of `el` (an element, or a ref to one set before layout). */
export function useSize<E extends HTMLElement, T>(el: E | RefObject<E | null> | null, read: (el: E) => T, initial: T): T {
  const [value, setValue] = useState(initial);
  const latest = useRef(read);
  useLayoutEffect(() => {
    latest.current = read;
  });
  useLayoutEffect(() => {
    const node = el && 'current' in el ? el.current : el;
    if (!node) return;
    const ro = new ResizeObserver(() => setValue(latest.current(node)));
    ro.observe(node);
    return () => ro.disconnect();
  }, [el]);
  return value;
}
