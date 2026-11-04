import { useState, type RefObject } from 'react';

/** A drag handle between two areas. Dragging reports the pointer's offset into `container` along `axis`
 * and the container's size, both in px; arrow keys call onStep(-1 | 1). `percent` is for assistive tech.
 * The hit area is 8px around a 1px line, which thickens on hover and while dragging; during a drag the
 * page cannot be text-selected. */
export function Splitter({
  axis,
  container,
  percent,
  onMove,
  onStep,
  onDragging,
  label,
}: {
  axis: 'x' | 'y';
  container: RefObject<HTMLElement | null>;
  percent: number;
  onMove: (offset: number, size: number) => void;
  onStep: (dir: -1 | 1) => void;
  /** True while the pointer drags it, so the resized side can skip its animation and follow the pointer. */
  onDragging?: (dragging: boolean) => void;
  label: string;
}) {
  const [dragging, setDragging] = useState(false);
  const drag = (on: boolean) => {
    setDragging(on);
    document.body.style.userSelect = on ? 'none' : '';
    document.body.style.cursor = on ? (axis === 'x' ? 'col-resize' : 'row-resize') : '';
    onDragging?.(on);
  };
  const fromPointer = (e: React.PointerEvent) => {
    const r = container.current?.getBoundingClientRect();
    if (!r) return;
    if (axis === 'x') onMove(e.clientX - r.left, r.width);
    else onMove(e.clientY - r.top, r.height);
  };
  const back = axis === 'x' ? 'ArrowLeft' : 'ArrowUp';
  const fwd = axis === 'x' ? 'ArrowRight' : 'ArrowDown';
  return (
    <div
      role="separator"
      aria-label={label}
      aria-orientation={axis === 'x' ? 'vertical' : 'horizontal'}
      aria-valuenow={Math.round(percent)}
      tabIndex={0}
      onPointerDown={(e) => {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        drag(true);
      }}
      onLostPointerCapture={() => drag(false)}
      onPointerMove={(e) => e.currentTarget.hasPointerCapture(e.pointerId) && fromPointer(e)}
      onKeyDown={(e) => {
        if (e.key === back) onStep(-1);
        if (e.key === fwd) onStep(1);
      }}
      data-dragging={dragging || undefined}
      className={`group relative z-20 shrink-0 bg-border outline-none ${
        axis === 'x' ? 'w-px cursor-col-resize' : 'h-px cursor-row-resize'
      } after:absolute ${axis === 'x' ? 'after:inset-y-0 after:-inset-x-1' : 'after:inset-x-0 after:-inset-y-1'}`}
    >
      <span
        className={`pointer-events-none absolute bg-accent opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100 group-data-dragging:opacity-100 ${
          axis === 'x' ? 'inset-y-0 -left-px w-0.75' : 'inset-x-0 -top-px h-0.75'
        }`}
      />
    </div>
  );
}
