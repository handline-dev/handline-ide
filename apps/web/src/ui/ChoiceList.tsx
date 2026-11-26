// One choice from a few, as full-width rows: the picked row carries a check and the selected fill, so it never shows by colour
// alone. A `dim` row is shown faded and still takes the click (to say why it cannot be picked yet).
import { Check } from 'lucide-react';
import type { ReactNode } from 'react';

export interface ChoiceRow<T extends string> {
  value: T;
  label: ReactNode;
  /** A second line under the label. */
  detail?: ReactNode;
  /** Small tags after the label (what kind the choice is). */
  badges?: string[];
  /** A word at the row's end, before the check (why a dimmed row cannot be picked). */
  aside?: ReactNode;
  dim?: boolean;
}

export function ChoiceList<T extends string>({
  rows,
  value,
  onPick,
  label,
}: {
  rows: ChoiceRow<T>[];
  value: T | null;
  onPick: (v: T) => void;
  label: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} className="flex w-full flex-col overflow-hidden rounded-md border border-border">
      {rows.map((r, i) => {
        const on = value === r.value;
        return (
          <button
            key={r.value}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => onPick(r.value)}
            className={`flex w-full items-center gap-3 px-3 py-2.5 text-left text-sm outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-inset ${i ? 'border-t border-border' : ''} ${on ? 'bg-selected' : 'hover:bg-hover/30'}`}
          >
            <span className={`flex min-w-0 flex-1 flex-col ${r.dim ? 'opacity-50' : ''}`}>
              <span className="flex min-w-0 items-center gap-2">
                <span className="truncate text-fg">{r.label}</span>
                {r.badges?.map((b) => (
                  <span key={b} className="shrink-0 rounded-sm border border-muted/40 px-1.5 text-xs leading-5 text-muted">
                    {b}
                  </span>
                ))}
              </span>
              {r.detail && <span className="truncate text-xs text-muted">{r.detail}</span>}
            </span>
            {r.aside && <span className="shrink-0 text-xs text-muted">{r.aside}</span>}
            <Check size={16} className={`shrink-0 text-accent ${on ? '' : 'invisible'}`} aria-hidden />
          </button>
        );
      })}
    </div>
  );
}
