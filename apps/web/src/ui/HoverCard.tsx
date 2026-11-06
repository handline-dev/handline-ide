import { PreviewCard as P } from '@base-ui/react/preview-card';
import { cloneElement, useEffect, useId, useRef, useState, type MouseEvent, type ReactElement, type ReactNode } from 'react';
import { floatingSurface } from '#src/ui/Popover.tsx';

/** A panel that opens while the pointer rests on `trigger` and stays while it moves into the panel. Its contents can be used, as a
 * menu's can. A click on the trigger is the trigger's own (the branch opens its diff); with `pin`, a click instead keeps the panel
 * open until a click outside it or Escape, for an item whose panel is all it has (the account). With `off` it never opens, so a
 * trigger whose panel may come and go keeps its place in the tree. */
export function HoverCard({
  trigger,
  children,
  align = 'start',
  pin = false,
  off = false,
  side = 'top',
}: {
  trigger: ReactElement<{ onClick?: (e: MouseEvent) => void; 'data-pin'?: string }>;
  children: ReactNode;
  align?: 'start' | 'center' | 'end';
  pin?: boolean;
  off?: boolean;
  side?: 'top' | 'right';
}) {
  const [hovered, setHovered] = useState(false);
  const [pinned, setPinned] = useState(false);
  const popup = useRef<HTMLDivElement>(null);
  const id = useId();
  useEffect(() => {
    if (!pinned) return;
    const outside = (e: PointerEvent) => {
      // The trigger's own click toggles it; anything else outside the panel closes it.
      const at = e.target as Element;
      if (!popup.current?.contains(at) && !at.closest(`[data-pin="${id}"]`)) setPinned(false);
    };
    const escape = (e: KeyboardEvent) => e.key === 'Escape' && setPinned(false);
    // After the click that pinned it, so that click is not taken as outside.
    const t = setTimeout(() => document.addEventListener('pointerdown', outside));
    document.addEventListener('keydown', escape);
    return () => {
      clearTimeout(t);
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  }, [pinned, id]);
  const shown = pin
    ? cloneElement(trigger, {
        'data-pin': id,
        onClick: (e: MouseEvent) => {
          trigger.props.onClick?.(e);
          setPinned(!pinned);
        },
      })
    : trigger;
  return (
    <P.Root open={!off && (hovered || pinned)} onOpenChange={setHovered}>
      <P.Trigger render={shown} delay={300} closeDelay={150} />
      <P.Portal>
        <P.Positioner side={side} align={align} sideOffset={6} collisionPadding={8} className="z-50">
          <P.Popup ref={popup} className={`${floatingSurface} min-w-48 p-1`}>
            {children}
          </P.Popup>
        </P.Positioner>
      </P.Portal>
    </P.Root>
  );
}
