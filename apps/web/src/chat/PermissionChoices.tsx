// A permission ask's answers as the terminal lists them: one full-width row each, numbered, the number key choosing it while no field
// has the keys.
import { Loader2 } from 'lucide-react';
import { useEffect } from 'react';

export type Choice = { key: string; label: string };

export function PermissionChoices({
  choices,
  busy,
  locked,
  pick,
}: {
  choices: Choice[];
  busy: (key: string) => boolean;
  locked: boolean;
  pick: (key: string) => void;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (locked || e.metaKey || e.ctrlKey || e.altKey || t?.closest('input,textarea,[contenteditable="true"]')) return;
      const c = choices[Number(e.key) - 1];
      if (c) {
        e.preventDefault();
        pick(c.key);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [choices, locked, pick]);
  return (
    <div role="group" aria-label="Answer" className="flex flex-col">
      {choices.map((c, i) => (
        <button
          key={c.key}
          type="button"
          disabled={locked}
          onClick={() => pick(c.key)}
          className="flex min-h-9 items-center gap-3 rounded-md px-2 text-left hover:bg-hover focus-visible:bg-hover disabled:opacity-60 disabled:hover:bg-transparent"
        >
          <span className="w-3 shrink-0 font-mono text-xs text-muted">{i + 1}</span>
          <span className="min-w-0 flex-1 break-words text-fg">{c.label}</span>
          {busy(c.key) && <Loader2 size={14} className="shrink-0 animate-spin text-muted" aria-label="Sending" />}
        </button>
      ))}
    </div>
  );
}
