import { Info } from 'lucide-react';
import type { ReactNode } from 'react';
import { Popover } from '#src/ui/Popover.tsx';

/** An (i) that opens `children` on click: a setting's explanation, kept out of the way until asked for. */
export function InfoTip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Popover
      trigger={
        <button type="button" aria-label={label} className="inline-grid size-4 place-items-center rounded-full text-muted hover:text-fg">
          <Info size={13} />
        </button>
      }
    >
      <div className="max-w-64 text-xs text-muted">{children}</div>
    </Popover>
  );
}
