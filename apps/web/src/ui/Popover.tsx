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
  /** The bar it opens flush on: its bottom on the bar's top edge, its end on the bar's end, with no gap. */
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
          sideOffset={corner ? 0 : 4}
          collisionPadding={corner ? 0 : 8}
          anchor={corner}
          className="z-50"
        >
          <P.Popup className={`${floatingSurface} p-3 text-sm ${corner ? 'rounded-b-none rounded-tr-none border-r-0 border-b-0' : ''}`}>
            {children}
          </P.Popup>
        </P.Positioner>
      </P.Portal>
    </P.Root>
  );
}
