// The city's Find field, in a corner of its pane: typing lists the teams and people that match (find.ts), a person with its team; the
// arrow keys and Enter, or a click, pick one. With nothing typed, or nothing matching, it lists the row that adds a team. Escape empties
// it and gives the keys back to walking. Its keys are its own: the founder stands still while it has them. On a narrow pane it is an
// icon until it has the keys, so it covers nothing of the city.
import { Plus, Search } from 'lucide-react';
import { useState, type KeyboardEvent, type RefObject } from 'react';
import { NEW_TEAM, rowsFor, type Found } from '#src/town/find.ts';

const WORDS = 'Find a team or person';

export function Find({
  field,
  houses,
  compact,
  onPick,
}: {
  /** The field itself, so a key pressed outside can bring the keys to it. */
  field: RefObject<HTMLInputElement | null>;
  houses: Parameters<typeof rowsFor>[0];
  /** The pane is narrow: only the icon shows until the field has the keys or holds a word. */
  compact: boolean;
  onPick: (found: Found) => void;
}) {
  const [query, setQuery] = useState('');
  const [at, setAt] = useState(0);
  const [mine, setMine] = useState(false);
  const rows = rowsFor(houses, query);
  const sel = Math.min(at, rows.length - 1);
  const none = query.trim() !== '' && rows[0] === NEW_TEAM;
  const open = !compact || mine || query !== '';
  const leave = () => {
    setQuery('');
    setAt(0);
    field.current?.blur();
  };
  const pick = (f: Found) => {
    leave();
    onPick(f);
  };
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.nativeEvent.isComposing) return; // a Korean syllable still being made keeps its Enter and arrows
    if (e.key === 'Escape') leave();
    else if (e.key === 'Enter') pick(rows[sel]);
    else if (e.key === 'ArrowDown' || e.key === 'ArrowUp')
      setAt((a) => (Math.min(a, rows.length - 1) + (e.key === 'ArrowDown' ? 1 : rows.length - 1)) % rows.length);
    else return;
    e.preventDefault();
    e.stopPropagation();
  };
  return (
    <div className="pointer-events-none absolute inset-x-3 top-3 z-10 flex">
      <div data-no-walk className={`pointer-events-auto flex max-w-full flex-col gap-1 ${open ? 'w-60' : ''}`}>
        <label
          title={open ? undefined : WORDS}
          className={`flex h-7 items-center rounded-md border border-border bg-bg text-sm shadow-popover focus-within:border-accent ${
            open ? 'gap-1.5 px-2' : 'w-7 cursor-pointer justify-center hover:bg-hover'
          }`}
        >
          <Search size={14} className="shrink-0 text-muted" />
          <input
            ref={field}
            value={query}
            placeholder={WORDS}
            aria-label={WORDS}
            className={`bg-transparent text-fg outline-none placeholder:text-muted ${open ? 'min-w-0 flex-1' : 'w-0 opacity-0'}`}
            onChange={(e) => {
              setQuery(e.target.value);
              setAt(0);
            }}
            onFocus={() => setMine(true)}
            onBlur={() => setMine(false)}
            onKeyDown={onKeyDown}
          />
          {open && !mine && (
            <kbd aria-hidden className="rounded-sm border border-border px-1 font-sans text-xs text-muted">
              /
            </kbd>
          )}
        </label>
        {mine && (
          <div className="rounded-lg border border-border bg-bg py-1 shadow-popover">
            {none && <p className="px-3 py-1 text-xs text-muted">No team or person matches.</p>}
            <ul role="listbox" aria-label={none || query.trim() === '' ? 'What you can do' : 'Matches'}>
              {rows.map((f, i) => (
                <li key={`${f.house}|${f.person ?? ''}`} role="option" aria-selected={i === sel}>
                  <button
                    type="button"
                    onMouseDown={(e) => e.preventDefault()}
                    onMouseEnter={() => setAt(i)}
                    onClick={() => pick(f)}
                    className={`flex h-7 w-full items-center gap-2 px-3 text-left text-sm ${i === sel ? 'bg-hover' : ''}`}
                  >
                    {f === NEW_TEAM && <Plus size={14} className="shrink-0 text-accent" />}
                    <span className={`min-w-0 truncate ${f === NEW_TEAM ? 'text-accent' : 'text-fg'}`}>{f.name}</span>
                    <span className="min-w-0 flex-1 truncate text-right text-xs text-muted">
                      {f === NEW_TEAM ? '' : f.team ? `in ${f.team}` : 'Team'}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  );
}
