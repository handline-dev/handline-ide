import { useState, type ReactElement } from 'react';
import { Input } from '#src/ui/Field.tsx';
import { Popover } from '#src/ui/Popover.tsx';

/** Picks one of `everyone`: a search over all of them, and before typing the short list `likely`. A click or Enter picks; arrows move.
 *  `label` names one, `words` is what a search looks in (its label by default), `note` says something short beside it. */
export function NamePicker({
  trigger,
  everyone,
  likely,
  label,
  words = label,
  note,
  placeholder = 'Find someone',
  onPick,
  onOpen,
}: {
  trigger: ReactElement;
  everyone: string[];
  likely: string[];
  label: (name: string) => string;
  words?: (name: string) => string;
  note?: (name: string) => string | undefined;
  placeholder?: string;
  onPick: (name: string) => void;
  onOpen?: () => void;
}) {
  const [shown, setShown] = useState(false);
  const [find, setFind] = useState('');
  const [at, setAt] = useState(0);
  const q = find.trim().toLowerCase();
  const names = q ? everyone.filter((n) => words(n).toLowerCase().includes(q)).slice(0, 8) : likely;
  const pick = names[Math.min(at, names.length - 1)];
  const choose = (to = pick) => {
    setShown(false);
    if (to) onPick(to);
  };
  return (
    <Popover
      open={shown}
      onOpenChange={(o) => {
        setShown(o);
        setFind('');
        setAt(0);
        if (o) onOpen?.();
      }}
      trigger={trigger}
    >
      <div className="flex w-64 flex-col gap-1">
        <Input
          autoFocus
          placeholder={placeholder}
          value={find}
          onChange={(e) => {
            setFind(e.target.value);
            setAt(0);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              choose();
            } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
              e.preventDefault();
              setAt((i) => (i + (e.key === 'ArrowDown' ? 1 : names.length - 1)) % Math.max(names.length, 1));
            }
          }}
        />
        <ul role="listbox" aria-label={q ? 'Matches' : 'Likely'} className="flex flex-col">
          {names.map((n) => (
            <li key={n} role="option" aria-selected={n === pick}>
              <button
                type="button"
                onMouseEnter={() => setAt(names.indexOf(n))}
                onClick={() => choose(n)}
                className={`flex h-7 w-full items-center justify-between gap-2 rounded-sm px-2 text-left ${n === pick ? 'bg-hover' : ''}`}
              >
                <span className="truncate">{label(n)}</span>
                {note?.(n) && <span className="shrink-0 text-xs text-muted">{note(n)}</span>}
              </button>
            </li>
          ))}
        </ul>
        {!names.length && <p className="px-2 text-xs text-muted">No matches</p>}
        {!q && everyone.length > names.length && <p className="px-2 text-xs text-muted">Type to find more · {everyone.length} in all</p>}
      </div>
    </Popover>
  );
}
