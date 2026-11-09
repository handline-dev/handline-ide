// The message box's two lists: `@` at the end of the text offers agents to name, `/` at the start of the line offers the box's commands.
// Arrows move, Enter or Tab picks, Escape closes. A `/` line is never sent as words: it runs its command, or says there is none.
import { useAgents } from '@handline/app-data';
import { AtSign, ChevronRight } from 'lucide-react';
import { useState, type KeyboardEvent, type ReactNode } from 'react';

/** A `/` command: `takes` names its argument (`/assign <name>`); run gets what follows the name. */
export type Command = { name: string; hint: string; takes?: string; icon?: ReactNode; run: (arg: string) => void };

type Entry = { key: string; label: string; hint: string; icon: ReactNode; pick: () => void };

const AT = /(?:^|\s)@([\w.-]*)$/;
const SLASH = /^\/(\w*)$/;
const LINE = /^\/(\w+)(?:\s+(.*))?$/;

export function useSuggest(draft: string, setDraft: (v: string) => void, commands: Command[]) {
  const [shut, setShut] = useState<string | null>(null); // not kept: closed until the text changes
  const [at, setAt] = useState(0);
  // Said for the text it was sent with; gone once the text changes.
  const [refused, setRefused] = useState<{ text: string; why: string } | null>(null);
  const agents = useAgents().data?.agents ?? [];

  const mention = AT.exec(draft)?.[1];
  const slash = SLASH.exec(draft)?.[1];
  const q = (mention ?? slash ?? '').toLowerCase();
  const all: Entry[] =
    mention !== undefined
      ? agents
          .filter((a) => a.name.toLowerCase().includes(q))
          .sort((a, b) => Number(!a.name.toLowerCase().startsWith(q)) - Number(!b.name.toLowerCase().startsWith(q)))
          .slice(0, 8)
          .map((a) => ({
            key: a.name,
            label: `@${a.name}`,
            hint: a.state,
            icon: <AtSign size={14} />,
            pick: () => setDraft(draft.replace(/@[\w.-]*$/, `@${a.name} `)),
          }))
      : slash !== undefined
        ? commands
            .filter((c) => c.name.startsWith(q))
            .map((c) => ({
              key: c.name,
              label: `/${c.name}${c.takes ? ` <${c.takes}>` : ''}`,
              hint: c.hint,
              icon: c.icon ?? <ChevronRight size={14} />,
              // One that takes something waits for it in the box; one that does not runs now.
              pick: () => (c.takes ? setDraft(`/${c.name} `) : (setDraft(''), c.run(''))),
            }))
        : [];
  // Escape closes it for this text only: any change to the text (erasing included) opens it again.
  if (shut !== null && shut !== draft) setShut(null);
  const entries = shut === draft ? [] : all;
  const sel = Math.min(at, entries.length - 1);

  /** Arrows, Enter, Tab and Escape while the list is open; true when the key was the list's. */
  const onKeyDown = (e: KeyboardEvent) => {
    if (!entries.length || e.nativeEvent.isComposing) return false;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') setAt((sel + (e.key === 'ArrowDown' ? 1 : entries.length - 1)) % entries.length);
    else if (e.key === 'Enter' || e.key === 'Tab') {
      entries[sel]!.pick();
      setAt(0);
    } else if (e.key === 'Escape') setShut(draft);
    else return false;
    e.preventDefault();
    return true;
  };

  /** A `/` line in place of a send: runs its command and clears the box, or says there is none. True when it was one. */
  const command = (text: string) => {
    const m = LINE.exec(text.trim());
    if (!m) return false;
    const c = commands.find((x) => x.name === m[1]);
    if (!c) {
      setRefused({ text, why: `No command /${m[1]}. Type / to see them.` });
      return true;
    }
    if (c.takes && !m[2]?.trim()) {
      setRefused({ text, why: `/${c.name} needs ${c.takes}.` });
      return true;
    }
    setRefused(null);
    setDraft('');
    c.run(m[2]?.trim() ?? '');
    return true;
  };

  const menu = entries.length > 0 && (
    <ul
      role="listbox"
      aria-label={mention !== undefined ? 'Agents' : 'Commands'}
      className="absolute inset-x-0 bottom-full z-10 mb-1 max-h-60 overflow-auto rounded-lg border border-border bg-bg py-1 shadow-popover"
    >
      {entries.map((e, i) => (
        <li key={e.key} role="option" aria-selected={i === sel}>
          <button
            type="button"
            onMouseDown={(ev) => ev.preventDefault()}
            onMouseEnter={() => setAt(i)}
            onClick={() => e.pick()}
            className={`flex h-8 w-full items-center gap-2 px-3 text-left text-sm ${i === sel ? 'bg-hover' : ''}`}
          >
            <span className="grid size-4 shrink-0 place-items-center text-muted">{e.icon}</span>
            <span className="shrink-0 text-fg">{e.label}</span>
            <span className="min-w-0 truncate text-muted">{e.hint}</span>
          </button>
        </li>
      ))}
    </ul>
  );

  // A `/word` no command starts with is said as it is typed, not only when sent.
  const unknown = slash && !commands.some((c) => c.name.startsWith(slash)) ? `No command /${slash}. Type / to see them.` : null;
  return { menu, onKeyDown, command, refused: unknown ?? (refused?.text === draft ? refused.why : null) };
}
