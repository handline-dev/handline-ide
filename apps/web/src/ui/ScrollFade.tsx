import { useEffect, useRef, useState, type ComponentProps, type CSSProperties, type RefObject } from 'react';

const FADE = 24; // px

type Edges = { top: boolean; bottom: boolean; left: boolean; right: boolean };
const none: Edges = { top: false, bottom: false, left: false, right: false };

/** One axis of the mask: each end fades only while there is more to scroll that way. */
const ramp = (dir: string, start: boolean, end: boolean) =>
  `linear-gradient(${dir}, transparent 0, black ${start ? FADE : 0}px, black calc(100% - ${end ? FADE : 0}px), transparent 100%)`;

/** The style that fades `ref`'s edges, on each of its four sides, while there is more to scroll that way.
 * For a scroll area that has its own ref; ScrollFade is the same as a plain box. */
export function useScrollFade(ref: RefObject<HTMLElement | null>): CSSProperties {
  const [edges, setEdges] = useState(none);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const read = () =>
      setEdges((e) => {
        const n = {
          top: el.scrollTop > 1,
          bottom: el.scrollTop + el.clientHeight < el.scrollHeight - 1,
          left: el.scrollLeft > 1,
          right: el.scrollLeft + el.clientWidth < el.scrollWidth - 1,
        };
        return (Object.keys(n) as (keyof Edges)[]).every((k) => e[k] === n[k]) ? e : n;
      });
    read();
    el.addEventListener('scroll', read, { passive: true });
    // Content that grows or shrinks (an image loading, text arriving) changes what is left to scroll.
    const resize = new ResizeObserver(read);
    resize.observe(el);
    const children = new MutationObserver(() => {
      for (const c of el.children) resize.observe(c);
      read();
    });
    children.observe(el, { childList: true });
    for (const c of el.children) resize.observe(c);
    return () => {
      el.removeEventListener('scroll', read);
      resize.disconnect();
      children.disconnect();
    };
  }, [ref]);
  // The two axes multiply: a corner fades where either does.
  return {
    maskImage: `${ramp('to bottom', edges.top, edges.bottom)}, ${ramp('to right', edges.left, edges.right)}`,
    maskComposite: 'intersect',
  };
}

/** A scroll area whose edges fade while there is more to scroll that way. */
export function ScrollFade({ className = '', style, children, ...props }: ComponentProps<'div'>) {
  const ref = useRef<HTMLDivElement>(null);
  const fade = useScrollFade(ref);
  return (
    <div ref={ref} className={`overflow-auto ${className}`} style={{ ...fade, ...style }} {...props}>
      {children}
    </div>
  );
}
