// A command shown before a decision, one way everywhere: whole, wrapped at spaces (only a word longer than the line breaks, never a
// scroll or a cut), past FOLD
// lines the rest behind "Show all" that opens in place.
import { useState } from 'react';

/** Lines shown before the rest fold. */
const FOLD = 12;

export function CommandText({ text, folder }: { text: string; folder?: string }) {
  const [all, setAll] = useState(false);
  const lines = text.split('\n');
  const folded = !all && lines.length > FOLD;
  return (
    <div className="min-w-0 overflow-hidden rounded-md border border-border bg-surface">
      <pre className="px-3 py-2 font-mono text-xs leading-5 [overflow-wrap:anywhere] whitespace-pre-wrap text-fg">
        {folded ? lines.slice(0, FOLD).join('\n') : text}
      </pre>
      {folded && (
        <button
          type="button"
          onClick={() => setAll(true)}
          className="w-full border-t border-border px-3 py-1 text-left text-xs text-muted hover:text-fg"
        >
          Show all {lines.length} lines
        </button>
      )}
      {folder && <p className="border-t border-border px-3 py-1 font-mono text-xs break-all text-muted">in {folder}</p>}
    </div>
  );
}
