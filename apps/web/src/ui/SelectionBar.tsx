import { X } from 'lucide-react';
import type { ReactNode } from 'react';
import { IconButton } from '#src/ui/IconButton.tsx';

/** What a list's scroll area keeps free under its last row while the bar floats over it, so no row stays hidden behind it. */
export const selectionRoom = (count: number) => (count > 0 ? 'pb-16 scroll-pb-16' : '');

/** A list whose rows can be selected, with the bar that floats at its foot while any are: how many, what can be done to them all,
 * and a way out (the ×, or Escape inside the list). The scroll area in `children` takes `selectionRoom(count)`. */
export function WithSelectionBar({
  count,
  onClear,
  actions,
  className = '',
  children,
}: {
  count: number;
  onClear: () => void;
  actions: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      className={`relative flex min-h-0 min-w-0 flex-col ${className}`}
      onKeyDown={(e) => {
        if (e.key !== 'Escape' || count === 0 || e.defaultPrevented) return;
        e.preventDefault();
        onClear();
      }}
    >
      {children}
      {count > 0 && (
        <div className="pointer-events-none absolute inset-x-0 bottom-4 z-20 flex justify-center px-4">
          <div
            role="region"
            aria-label="Selection"
            className="pointer-events-auto flex max-w-full items-center gap-1 rounded-lg border border-border bg-bg p-1 pl-3 text-sm shadow-popover transition duration-150 starting:translate-y-2 starting:opacity-0"
          >
            <span className="tabular-nums whitespace-nowrap text-fg">{`${count} selected`}</span>
            <span className="mx-1.5 h-4 w-px shrink-0 bg-border" />
            {actions}
            <IconButton label="Clear the selection (Esc)" onClick={onClear}>
              <X size={14} />
            </IconButton>
          </div>
        </div>
      )}
    </div>
  );
}
