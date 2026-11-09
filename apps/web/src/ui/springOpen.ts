import { useEffect, useRef, useState, type CSSProperties, type DragEvent } from 'react';
import { carriesFiles } from '#src/ui/fileDrop.tsx';

/** How long dragged files are held over a row or a tab before it opens, in ms. */
export const HOLD = 700;

/** Spring-loading, as a file manager does it: hold dragged files over a row or a tab for a moment and it opens, so the files can be
 * dropped where it leads. Spread `props` on it: while the hold runs it has data-spring, and an outline fills around it (styles.css). */
export function useSpringOpen(open: () => void) {
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const depth = useRef(0); // dragenter and dragleave also fire for the row's children
  const [hovering, setHovering] = useState(false);
  const stop = () => {
    clearTimeout(timer.current);
    depth.current = 0;
    setHovering(false);
  };
  useEffect(() => () => clearTimeout(timer.current), []);
  return {
    props: {
      'data-spring': hovering || undefined,
      style: { '--spring-hold': `${HOLD}ms` } as CSSProperties,
      onDragEnter: (e: DragEvent) => {
        if (!carriesFiles(e)) return;
        // Its own statement: React Compiler 1.0 turns a ref's `x.current++` inside a comparison into a pre-increment.
        depth.current += 1;
        if (depth.current > 1) return;
        setHovering(true);
        timer.current = setTimeout(() => {
          stop();
          open();
        }, HOLD);
      },
      onDragLeave: (e: DragEvent) => {
        if (!carriesFiles(e)) return;
        depth.current -= 1;
        if (depth.current <= 0) stop();
      },
      onDrop: stop,
    },
  };
}
