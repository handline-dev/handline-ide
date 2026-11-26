// A tab bar floating over the town (Windows.tsx): one tab bar on its own follows its handle as it is dragged and stays where it is let
// go; one split in two carries its title instead, as a docked one does. It never leaves the town's area.
import { motion, useMotionValue } from 'framer-motion';
import { useLayoutEffect, useRef } from 'react';
import { Workspace, type Dispatch, type IconFor } from '#src/Workspace.tsx';
import type { GripProps } from '#src/layout/paneGrip.tsx';
import { holds, panes, type Pane } from '#src/layout/tree.ts';
import type { Float } from '#src/town/windows.ts';

export const FLOAT_W = 400;
const FLOAT_H = 560;
/** As tall as it may be, and never past the town's bottom edge. */
const HEIGHT = `min(${FLOAT_H}px, calc(100% - 24px))`;

export function FloatWindow({
  float: f,
  area,
  focus,
  onFocus,
  dispatch,
  iconFor,
  grip,
  onMoved,
}: {
  float: Float;
  area: () => DOMRect;
  focus: string;
  onFocus: (pane: string) => void;
  dispatch: Dispatch;
  iconFor: IconFor;
  /** `moved` keeps the float's own move when it is let go on nothing that takes a tab bar. */
  grip: (pane: Pane, moved?: () => void) => Omit<GripProps, 'label'>;
  onMoved: (x: number, y: number) => void;
}) {
  // While its handle is dragged it follows the pointer by these offsets, which draw without rendering the tabs again.
  const dx = useMotionValue(0);
  const dy = useMotionValue(0);
  const box = useRef<HTMLDivElement>(null);
  const single = f.root.kind === 'pane';
  const home = () => {
    dx.set(0);
    dy.set(0);
  };
  // Let go, it rests where it was carried: its place is kept, and the offsets go back to none as it is drawn there.
  useLayoutEffect(home, [f.x, f.y, dx, dy]);
  const moved = () => {
    const r = area();
    const x = Math.min(1 - FLOAT_W / r.width, Math.max(0, f.x + dx.get() / r.width));
    const y = Math.min(0.9, Math.max(0, f.y + dy.get() / r.height));
    if (x === f.x && y === f.y) home();
    else onMoved(x, y);
  };
  return (
    <motion.div
      ref={box}
      data-no-walk
      initial={{ opacity: 0, scale: 0.96 }}
      animate={{ opacity: 1, scale: 1 }}
      exit={{ opacity: 0, scale: 0.96 }}
      transition={{ duration: 0.14, ease: 'easeOut' }}
      onPointerDownCapture={() => onFocus(focus && holds(f.root, focus) ? focus : firstPane(f))}
      style={{
        x: dx,
        y: dy,
        left: `clamp(0px, ${f.x * 100}%, max(0px, calc(100% - ${FLOAT_W}px)))`,
        top: `clamp(0px, ${f.y * 100}%, calc(100% - ${HEIGHT}))`,
        width: `min(${FLOAT_W}px, 100%)`,
        height: HEIGHT,
      }}
      className="absolute z-20 flex overflow-hidden rounded-lg border border-border bg-bg shadow-xl"
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
          const g = grip(pane, moved);
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
              if (!at) return;
              dx.set(at.x);
              dy.set(at.y);
            },
          };
        }}
      />
    </motion.div>
  );
}

const firstPane = (f: Float) => panes(f.root)[0].id;
