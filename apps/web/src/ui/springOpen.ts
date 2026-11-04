import { useEffect, useRef, useState, type DragEvent } from 'react';
import { carriesFiles } from '#src/ui/fileDrop.tsx';

const HOLD = 700; // ms

/** Spring-loading, as a file manager does it: hold dragged files over a row for a moment and it opens, so the
 * files can be dropped where it leads. `hovering` is true while the files are over it, for a highlight. */
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
    hovering,
    handlers: {
      onDragEnter: (e: DragEvent) => {
        if (!carriesFiles(e) || depth.current++ > 0) return;
        setHovering(true);
        timer.current = setTimeout(() => {
          stop();
          open();
        }, HOLD);
      },
      onDragLeave: (e: DragEvent) => {
        if (carriesFiles(e) && --depth.current <= 0) stop();
      },
      onDrop: stop,
    },
  };
}
