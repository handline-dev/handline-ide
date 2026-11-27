import { Popover as P } from '@base-ui/react/popover';
import type { ReactElement, ReactNode, RefObject } from 'react';

type Side = 'top' | 'bottom' | 'left' | 'right';
type Align = 'start' | 'center' | 'end';

/** Every floating panel (popover, menu) has this surface: one look, one elevation, one entry motion. */
export const floatingSurface =
  'rounded-md border border-border bg-bg text-fg shadow-popover outline-none origin-(--transform-origin) transition duration-100 data-[starting-style]:scale-95 data-[starting-style]:opacity-0 data-[ending-style]:scale-95 data-[ending-style]:opacity-0';

/** A panel anchored to `trigger`. Opens on click, closes on outside click or Escape, keeps to the viewport. */
export function Popover({
  trigger,
  side = 'bottom',
  align = 'start',
  open,
  onOpenChange,
  corner,
  children,
}: {
  trigger: ReactElement;
  side?: Side;
  align?: Align;
  /** Held by the caller, to close it from inside (a choice made). */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  /** The bar it opens from: it floats a gap above the bar, inset from the window's edge. */
  corner?: RefObject<HTMLElement | null>;
  children: ReactNode;
}) {
  return (
    <P.Root open={open} onOpenChange={onOpenChange && ((o) => onOpenChange(o))}>
      <P.Trigger render={trigger} />
      <P.Portal>
        <P.Positioner
          side={side}
          align={align}
          sideOffset={corner ? 12 : 4}
          collisionPadding={8}
          anchor={corner}
          // In the corner it never flips: at page zoom the bar ends a fraction of a pixel past the window, which would flip it to the left.
          collisionAvoidance={corner ? { side: 'none', align: 'shift', fallbackAxisSide: 'none' } : undefined}
          className="z-50"
        >
          <P.Popup className={`${floatingSurface} p-3 text-sm ${corner ? 'max-h-(--available-height) overflow-y-auto' : ''}`}>
            {children}
          </P.Popup>
        </P.Positioner>
      </P.Portal>
    </P.Root>
  );
}
