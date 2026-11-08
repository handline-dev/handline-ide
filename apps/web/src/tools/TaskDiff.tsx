import { ago } from '#src/format/time.ts';

export interface Change {
  from: string;
  to: string;
  at: string;
}

type Line = { text: string; kind: 'same' | 'del' | 'add' };

/** Line by line, as a plain-text diff: what stayed, what went, what came, in the order of the new text. */
export function diffLines(a: string, b: string): Line[] {
  const x = a.split('\n');
  const y = b.split('\n');
  const lcs = Array.from({ length: x.length + 1 }, () => new Array<number>(y.length + 1).fill(0));
  for (let i = x.length - 1; i >= 0; i--)
    for (let j = y.length - 1; j >= 0; j--)
      lcs[i]![j] = x[i] === y[j] ? lcs[i + 1]![j + 1]! + 1 : Math.max(lcs[i + 1]![j]!, lcs[i]![j + 1]!);
  const out: Line[] = [];
  let i = 0;
  let j = 0;
  while (i < x.length || j < y.length) {
    if (i < x.length && j < y.length && x[i] === y[j]) {
      out.push({ kind: 'same', text: x[i++]! });
      j++;
    } else if (j >= y.length || (i < x.length && lcs[i + 1]![j]! >= lcs[i]![j + 1]!)) out.push({ kind: 'del', text: x[i++]! });
    else out.push({ kind: 'add', text: y[j++]! });
  }
  return out;
}

const MARK = { same: ' ', del: '-', add: '+' } as const;
const TONE = { same: '', del: 'bg-danger/15 text-danger', add: 'bg-success/15 text-success' } as const;

/** Each rewrite, oldest first: its lines, removed in red with a minus, added in green with a plus, unchanged plain; and when it was made. */
export function Changes({ changes }: { changes: Change[] }) {
  if (!changes.length) return null;
  return (
    <div className="mt-1 flex flex-col gap-2 border-l-2 border-border pl-3 text-xs text-muted">
      {changes.map((c, n) => (
        <div key={n} className="flex flex-col">
          {diffLines(c.from, c.to).map((l, k) => (
            <span key={k} className={`flex gap-2 whitespace-pre-wrap ${TONE[l.kind]}`}>
              <span aria-hidden className="select-none">
                {MARK[l.kind]}
              </span>
              <span className="min-w-0 flex-1">{l.text}</span>
            </span>
          ))}
          <span className="mt-0.5">{ago(c.at)}</span>
        </div>
      ))}
    </div>
  );
}
