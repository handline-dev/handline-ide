// Two or three large cards side by side, each an icon over a few words: a choice (`value` set, one checked) or a set of actions.
import { Check, type LucideIcon } from 'lucide-react';

export interface Card<T extends string> {
  value: T;
  icon: LucideIcon;
  label: string;
}

/** A choice: the picked card carries a check as well as its border, so it never shows by colour alone. Without `value` they are
 * plain buttons, each acting at once. */
export function ChoiceCards<T extends string>({
  cards,
  value,
  onPick,
  label,
}: {
  cards: Card<T>[];
  value?: T | null;
  onPick: (v: T) => void;
  label: string;
}) {
  const choice = value !== undefined;
  return (
    <div role={choice ? 'radiogroup' : 'group'} aria-label={label} className="grid auto-cols-fr grid-flow-col gap-3">
      {cards.map((c) => {
        const on = choice && value === c.value;
        return (
          <button
            key={c.value}
            type="button"
            role={choice ? 'radio' : undefined}
            aria-checked={choice ? on : undefined}
            onClick={() => onPick(c.value)}
            className={`relative flex h-28 flex-col items-center justify-center gap-3 rounded-md border px-3 text-center text-sm text-fg outline-none focus-visible:ring-2 focus-visible:ring-accent ${on ? 'border-accent bg-selected' : 'border-border hover:bg-hover/30'}`}
          >
            <c.icon size={28} strokeWidth={1.5} className={on ? 'text-accent' : 'text-muted'} />
            {c.label}
            <Check size={14} className={`absolute top-2 right-2 text-accent ${on ? '' : 'invisible'}`} />
          </button>
        );
      })}
    </div>
  );
}
