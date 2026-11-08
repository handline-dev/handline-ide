import { useState } from 'react';
import { ago } from '#src/format/time.ts';

export interface Change {
  from: string;
  to: string;
  at: string;
}

type Part = { text: string; kind: 'same' | 'del' | 'add' };

/** Word by word, what stayed, what went and what came: the longest common run of words, the rest marked. */
export function diffWords(a: string, b: string): Part[] {
  const x = a.match(/\s+|\S+/g) ?? [];
  const y = b.match(/\s+|\S+/g) ?? [];
  const n = x.length;
  const m = y.length;
  const lcs = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--)
    for (let j = m - 1; j >= 0; j--) lcs[i]![j] = x[i] === y[j] ? lcs[i + 1]![j + 1]! + 1 : Math.max(lcs[i + 1]![j]!, lcs[i]![j + 1]!);
  const out: Part[] = [];
  const push = (kind: Part['kind'], text: string) => {
    const last = out.at(-1);
    if (last?.kind === kind) last.text += text;
    else out.push({ kind, text });
  };
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      push('same', x[i++]!);
      j++;
    } else if (lcs[i + 1]![j]! >= lcs[i]![j + 1]!) push('del', x[i++]!);
    else push('add', y[j++]!);
  }
  while (i < n) push('del', x[i++]!);
  while (j < m) push('add', y[j++]!);
  return out;
}

/** Each rewrite, oldest first, under the words: what it took out in red and struck, what it put in in green; Diff / Plain shows the texts whole instead. */
export function Changes({ changes }: { changes: Change[] }) {
  const [plain, setPlain] = useState(false);
  if (!changes.length) return null;
  return (
    <div className="mt-1 flex flex-col gap-1 border-l-2 border-border pl-3 text-xs text-muted">
      <button type="button" onClick={() => setPlain(!plain)} className="w-fit hover:text-fg">
        {plain ? 'Show changes' : 'Show texts'}
      </button>
      {changes.map((c, n) => (
        <span key={n} className="whitespace-pre-wrap">
          {plain
            ? c.to
            : diffWords(c.from, c.to).map((p, k) => (
                <span
                  key={k}
                  className={
                    p.kind === 'del' ? 'bg-danger/15 text-danger line-through' : p.kind === 'add' ? 'bg-success/15 text-success' : ''
                  }
                >
                  {p.text}
                </span>
              ))}{' '}
          · {ago(c.at)}
        </span>
      ))}
    </div>
  );
}
