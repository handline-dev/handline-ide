import { Minus, Plus } from 'lucide-react';
import { useState } from 'react';

/**
 * A whole number from min to max: − and + step it and it can be typed (Enter or leaving the box commits, Escape drops what was typed).
 * `zeroWord` is what the box reads at 0 when not typed in ("No limit"). onCommit gets the clamped number, only when it differs from `value`;
 * if it returns a promise the box shows the sent number until `value` moves or the promise fails.
 */
export function NumberStepper({
  value,
  min = 0,
  max = 99,
  onCommit,
  label,
  stepLabels = ['Fewer', 'More'],
  zeroWord,
  disabled,
}: {
  value: number;
  min?: number;
  max?: number;
  onCommit: (n: number) => void | Promise<unknown>;
  label: string;
  stepLabels?: [string, string];
  zeroWord?: string;
  disabled?: boolean;
}) {
  const [typed, setTyped] = useState<string | null>(null);
  const [sent, setSent] = useState<number | null>(null);
  const [seen, setSeen] = useState(value);
  if (seen !== value) {
    setSeen(value);
    setSent(null);
  }
  const shown = sent ?? value;
  const go = (n: number) => {
    const next = Math.min(max, Math.max(min, n));
    setTyped(null);
    if (next === shown) return;
    const done = onCommit(next);
    if (done) {
      setSent(next);
      done.catch(() => setSent(null));
    }
  };
  const commit = () => {
    const n = Number(typed);
    if (typed !== null && typed.trim() !== '' && Number.isInteger(n)) go(n);
    else setTyped(null);
  };
  const word = zeroWord && shown === 0 ? zeroWord : String(shown);
  return (
    <div className={`flex h-8 items-center rounded-md border border-border bg-bg ${disabled ? 'opacity-50' : ''}`}>
      <button
        type="button"
        aria-label={stepLabels[0]}
        disabled={disabled || shown <= min}
        onClick={() => go(shown - 1)}
        className="grid h-full w-8 place-items-center text-muted hover:text-fg disabled:opacity-40"
      >
        <Minus size={14} />
      </button>
      <input
        aria-label={label}
        inputMode="numeric"
        disabled={disabled}
        value={typed ?? word}
        onFocus={(e) => e.currentTarget.select()}
        onChange={(e) => setTyped(e.target.value.replace(/\D/g, '').slice(0, Math.max(2, String(max).length)))}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') commit();
          if (e.key === 'Escape') setTyped(null);
        }}
        className={`h-full border-x border-border bg-transparent text-center tabular-nums outline-none focus:bg-surface ${zeroWord ? 'w-20' : 'w-9'}`}
      />
      <button
        type="button"
        aria-label={stepLabels[1]}
        disabled={disabled || shown >= max}
        onClick={() => go(shown + 1)}
        className="grid h-full w-8 place-items-center text-muted hover:text-fg disabled:opacity-40"
      >
        <Plus size={14} />
      </button>
    </div>
  );
}
