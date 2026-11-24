// The one control for picking one of a few choices: a row of words, the one in force raised.
import type { ReactNode } from 'react';

/** A small switch of choices, the one in force raised. */
export function Segmented<T extends string>({
  value,
  options,
  word,
  onChange,
  label,
  icon,
  disabled,
}: {
  value: T;
  options: readonly T[];
  word: (t: T) => string;
  onChange: (t: T) => void;
  label: string;
  disabled?: boolean;
  /** An icon before each choice's word. */
  icon?: (t: T) => ReactNode;
}) {
  return (
    <div role="radiogroup" aria-label={label} className="flex w-fit gap-0.5 rounded-lg bg-surface p-1">
      {options.map((o) => (
        <button
          key={o}
          type="button"
          role="radio"
          aria-checked={o === value}
          disabled={disabled}
          onClick={() => onChange(o)}
          title={icon ? word(o) : undefined}
          className="flex h-6 items-center justify-center gap-1.5 rounded-sm px-2 text-xs text-muted enabled:hover:text-fg disabled:opacity-50 aria-checked:bg-bg aria-checked:text-fg"
        >
          {icon?.(o)}
          {/* With an icon, a narrow bar (its parent an @container) shows the icon alone and the word on hover. */}
          <span className={icon ? 'hidden @lg:inline' : undefined}>{word(o)}</span>
        </button>
      ))}
    </div>
  );
}
