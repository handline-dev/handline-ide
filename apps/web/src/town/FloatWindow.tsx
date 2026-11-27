// A tab bar floating over the town (Windows.tsx), the town's full height: one tab bar on its own follows its handle sideways as it is
// dragged and stays where it is let go; one split in two carries its title instead, as a docked one does. Either edge drags its width.
// It never leaves the town's area.
import { motion, useMotionValue, type MotionValue } from 'framer-motion';
import { useLayoutEffect, useRef, type PointerEvent } from 'react';
import { Workspace, type Dispatch, type IconFor } from '#src/Workspace.tsx';
import type { GripProps } from '#src/layout/paneGrip.tsx';
import { holds, panes, type Pane } from '#src/layout/tree.ts';
import { FLOAT_MIN, type Float } from '#src/town/windows.ts';

export function FloatWindow({
  float: f,
  area,
  focus,
  onFocus,
  dispatch,
  iconFor,
  grip,
  onPlaced,
}: {
  float: Float;
  area: () => DOMRect;
  focus: string;
  onFocus: (pane: string) => void;
  dispatch: Dispatch;
  iconFor: IconFor;
  /** `moved` keeps the float's own move when it is let go on nothing that takes a tab bar. */
  grip: (pane: Pane, moved?: () => void) => Omit<GripProps, 'label'>;
  /** Its new left edge, as a share of the town's width, and its width in px. */
  onPlaced: (x: number, w: number) => void;
}) {
  // While its handle or an edge is dragged it follows the pointer by these, which draw without rendering the tabs again.
  const dx = useMotionValue(0);
  const wide = useMotionValue(f.w);
  const box = useRef<HTMLDivElement>(null);
  const single = f.root.kind === 'pane';
  const home = () => {
    dx.set(0);
    wide.set(f.w);
  };
  // Let go, it rests where it was carried: its place is kept, and the offsets go back to none as it is drawn there.
  useLayoutEffect(home, [f.x, f.w, dx, wide]);
  /** Its left edge as drawn, a share of the town's width: kept inside a town that got narrower than where it was put. */
  const x0 = (width: number) => Math.min(f.x, Math.max(0, 1 - f.w / width));
  const placed = () => {
    const r = area();
    const w = Math.min(wide.get(), r.width);
    const x = Math.min(1 - w / r.width, Math.max(0, x0(r.width) + dx.get() / r.width));
    if (x === f.x && w === f.w) home();
    else onPlaced(x, w);
  };
  return (
    <motion.div
      ref={box}
      data-no-walk
      initial={{ opacity: 0, scale: 0.98 }}
      animate={{ opacity: 1, scale: 1 }}
      exit={{ opacity: 0, scale: 0.98 }}
      transition={{ duration: 0.14, ease: 'easeOut' }}
      onPointerDownCapture={() => onFocus(focus && holds(f.root, focus) ? focus : firstPane(f))}
      style={{
        x: dx,
        width: wide,
        left: `clamp(0px, ${f.x * 100}%, max(0px, calc(100% - ${f.w}px)))`,
        maxWidth: '100%',
      }}
      className="absolute inset-y-0 z-20 flex overflow-hidden border-x border-border bg-bg shadow-xl"
    >
      <Workspace
        root={f.root}
        agent={null}
        focus={focus}
        onFocus={onFocus}
        dispatch={dispatch}
        reopenClosed={() => {}}
        quiet={!holds(f.root, focus)}
        iconFor={iconFor}
        grip={(pane) => {
          if (!single) return grip(pane);
          const g = grip(pane, placed);
          return {
            ...g,
            // A drop that leaves it where it was (on nothing that takes it) draws it back home; one that moves it is reset as it lands.
            onDrop: (drop, at) => {
              g.onDrop(drop, at);
              requestAnimationFrame(home);
            },
            onMove: (at) => {
              // Carried, it lets the pointer through to what is under it, which says where it would land.
              if (box.current) box.current.style.pointerEvents = at ? 'none' : '';
              if (at) dx.set(at.x);
            },
          };
        }}
      />
      <Edge side="left" dx={dx} wide={wide} room={() => x0(area().width) * area().width} onDone={placed} />
      <Edge side="right" dx={dx} wide={wide} room={() => area().width * (1 - x0(area().width))} onDone={placed} />
    </motion.div>
  );
}

/** A float's left or right edge, dragged to widen or narrow it; `room` is how wide it may grow that way, in px. */
function Edge({
  side,
  dx,
  wide,
  room,
  onDone,
}: {
  side: 'left' | 'right';
  dx: MotionValue<number>;
  wide: MotionValue<number>;
  room: () => number;
  onDone: () => void;
}) {
  const from = useRef<{ x: number; w: number; most: number } | null>(null);
  const drag = (e: PointerEvent) => {
    const s = from.current;
    if (!s) return;
    const d = e.clientX - s.x;
    const w = Math.max(Math.min(FLOAT_MIN, s.w), Math.min(s.most, side === 'left' ? s.w - d : s.w + d));
    wide.set(w);
    if (side === 'left') dx.set(s.w - w);
  };
  return (
    <div
      aria-hidden
      onPointerDown={(e) => {
        e.preventDefault();
        e.stopPropagation();
        e.currentTarget.setPointerCapture(e.pointerId);
        const w = wide.get();
        from.current = { x: e.clientX, w, most: side === 'left' ? w + room() : room() };
        document.body.style.cursor = 'col-resize';
      }}
      onPointerMove={drag}
      onLostPointerCapture={() => {
        if (!from.current) return;
        from.current = null;
        document.body.style.cursor = '';
        onDone();
      }}
      className={`absolute inset-y-0 z-20 w-1.5 cursor-col-resize hover:bg-accent/40 ${side === 'left' ? 'left-0' : 'right-0'}`}
    />
  );
}

const firstPane = (f: Float) => panes(f.root)[0].id;
