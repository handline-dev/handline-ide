import { useRef, useState, type ComponentProps, type DragEvent } from 'react';

type Handlers = {
  onDragEnter: (e: DragEvent) => void;
  onDragOver: (e: DragEvent) => void;
  onDragLeave: (e: DragEvent) => void;
  onDrop: (e: DragEvent) => void;
};

export type DropOptions = {
  /** Which drags this place takes; any other passes through untouched. */
  accepts: (e: DragEvent) => boolean;
  /** Set on each move when given; else the browser's, from what the drag allows. */
  dropEffect?: 'copy' | 'move';
  /** On each move over it, for where in it the drop would land (a tab bar's slot, a pane's zone). */
  onOver?: (e: DragEvent) => void;
  /** When the drag leaves it or is dropped. */
  onLeave?: () => void;
  onDrop: (e: DragEvent) => void;
};

/** The receiving side of a drag: `over` while a drag it takes is held over it. dragenter and dragleave also fire for its children,
 *  so a count of them, not the pointer's target, says when the drag has left. */
export function useDropTarget({ accepts, dropEffect, onOver, onLeave, onDrop }: DropOptions): { over: boolean; handlers: Handlers } {
  const depth = useRef(0);
  const [over, setOver] = useState(false);
  const leave = () => {
    depth.current = 0;
    setOver(false);
    onLeave?.();
  };
  return {
    over,
    handlers: {
      onDragEnter: (e) => {
        if (!accepts(e)) return;
        // Its own statement: React Compiler 1.0 turns a ref's `x.current++` inside a comparison into a pre-increment.
        depth.current += 1;
        if (depth.current === 1) setOver(true);
      },
      onDragOver: (e) => {
        if (!accepts(e)) return;
        e.preventDefault();
        if (dropEffect) e.dataTransfer.dropEffect = dropEffect;
        onOver?.(e);
      },
      onDragLeave: (e) => {
        if (!accepts(e)) return;
        depth.current -= 1;
        if (depth.current <= 0) leave();
      },
      onDrop: (e) => {
        if (!accepts(e)) return;
        e.preventDefault();
        leave();
        onDrop(e);
      },
    },
  };
}

/** A div that takes drops: useDropTarget for places drawn in a list, where a hook per item cannot be called. data-over while held. */
export function DropArea({ accepts, dropEffect, onOver, onLeave, onDrop, ...props }: DropOptions & ComponentProps<'div'>) {
  const { over, handlers } = useDropTarget({ accepts, dropEffect, onOver, onLeave, onDrop });
  return <div {...props} {...handlers} data-over={over || undefined} />;
}
