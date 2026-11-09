// Answering a question that lays out choices, as Claude Code asks: the choices numbered, and last "Something else" to write instead.
// ↑ ↓ move, Enter or a number picks, Escape steps out of the answer.
import { ChevronRight, CornerDownRight, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { IconButton } from '#src/ui/IconButton.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

type Choice = { key: string; text: string };

export function ChoicePicker({
  by,
  options,
  onPick,
  onOther,
  onCancel,
}: {
  by: string;
  options: Choice[];
  onPick: (c: Choice) => void;
  onOther: () => void;
  onCancel: () => void;
}) {
  const rows = [...options.map((o) => o.text), 'Something else'];
  const [at, setAt] = useState(0);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => box.current?.focus(), []);
  const choose = (i: number) => (i < options.length ? onPick(options[i]!) : onOther());
  return (
    <div
      ref={box}
      tabIndex={0}
      role="listbox"
      aria-label={`Answer ${by}`}
      aria-activedescendant={`choice-${at}`}
      className="my-3 flex flex-col gap-1 rounded-xl border border-border bg-bg py-2 pr-2 pl-4 outline-none focus:border-accent"
      onKeyDown={(e) => {
        // Keys on the × are its own; a number with Cmd, Ctrl or Alt is the browser's.
        if (e.target !== e.currentTarget && e.key !== 'Escape') return;
        const n = e.metaKey || e.ctrlKey || e.altKey ? NaN : Number(e.key);
        // A number picks the choice it keys ("2) ..."), else the row it numbers.
        const keyed = options.findIndex((o) => o.key === e.key);
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') setAt((i) => (i + (e.key === 'ArrowDown' ? 1 : rows.length - 1)) % rows.length);
        else if (e.key === 'Enter') choose(at);
        else if (e.key === 'Escape') onCancel();
        else if (Number.isInteger(n) && keyed >= 0) choose(keyed);
        else if (Number.isInteger(n) && n >= 1 && n <= rows.length) choose(n - 1);
        else return;
        e.preventDefault();
      }}
    >
      <div className="-ml-2 flex min-w-0 items-center gap-1.5 text-xs text-muted">
        <CornerDownRight size={14} className="ml-1 shrink-0" />
        <span className="min-w-0 flex-1 truncate">{`Answering ${by}'s question`}</span>
        <IconButton className="size-6!" label="Drop the answer" onClick={onCancel}>
          <X size={14} />
        </IconButton>
      </div>
      {/* Each written out whole; a long list scrolls in its place. */}
      <ScrollFade className="-ml-2 flex max-h-56 flex-col gap-0.5 text-sm">
        {rows.map((text, i) => (
          <div
            key={i}
            id={`choice-${i}`}
            role="option"
            aria-selected={i === at}
            onMouseEnter={() => setAt(i)}
            onClick={() => choose(i)}
            className={`flex shrink-0 cursor-default items-start gap-2 rounded-md px-2 py-1.5 ${i === at ? 'bg-hover text-fg' : 'text-fg'}`}
          >
            <ChevronRight size={14} className={`mt-0.5 shrink-0 text-accent ${i === at ? '' : 'invisible'}`} />
            <span className="w-4 shrink-0 text-muted tabular-nums">{`${i + 1}.`}</span>
            <span className={`min-w-0 ${i === options.length ? 'text-muted' : ''}`}>{text}</span>
          </div>
        ))}
      </ScrollFade>
      <span className="text-xs text-muted">{`↑ ↓ to move · Enter or 1–${rows.length} to pick · Esc to step out`}</span>
    </div>
  );
}
