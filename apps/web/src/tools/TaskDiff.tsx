import { useState } from 'react';
import { Select } from '#src/ui/Field.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { ago } from '#src/format/time.ts';

/** One version of a text: its words in full, when it was written and by whom. */
export interface Version {
  text: string;
  at: string;
  by: string;
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

const labelOf = (v: Version) => `${v.by} · ${ago(v.at)}`;

function History({ versions }: { versions: Version[] }) {
  // Oldest first; From and To start at the one before the latest and the latest.
  const [from, setFrom] = useState(Math.max(0, versions.length - 2));
  const [to, setTo] = useState(versions.length - 1);
  const pick = (v: number, set: (n: number) => void, name: string) => (
    <label className="flex items-center gap-2">
      {name}
      <Select value={v} onChange={(e) => set(Number(e.target.value))}>
        {versions.map((x, i) => (
          <option key={i} value={i}>
            {labelOf(x)}
          </option>
        ))}
      </Select>
    </label>
  );
  return (
    <div className="flex max-h-96 flex-col gap-4 overflow-y-auto pb-4 text-sm">
      <div className="flex flex-wrap gap-4">
        {pick(from, setFrom, 'From')}
        {pick(to, setTo, 'To')}
      </div>
      <div className="flex flex-col">
        {from === to ? (
          <span className="text-muted">Pick two different versions.</span>
        ) : (
          diffLines(versions[from]!.text, versions[to]!.text).map((l, k) => (
            <span key={k} className={`flex gap-2 whitespace-pre-wrap ${TONE[l.kind]}`}>
              <span aria-hidden className="select-none">
                {MARK[l.kind]}
              </span>
              <span className="min-w-0 flex-1">{l.text}</span>
            </span>
          ))
        )}
      </div>
      <div className="flex flex-col gap-3 border-t border-border pt-3">
        {[...versions].reverse().map((v, k) => (
          <div key={k} className="flex flex-col">
            <span className="text-xs text-muted">
              {v.by} · <time title={v.at}>{ago(v.at)}</time>
            </span>
            <span className="whitespace-pre-wrap">{v.text}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

/** The history of an edited text in a window of its own: every version in full, and the difference between any two picked (default: the
 * one before the latest and the latest) as lines, removed in red with a minus, added in green with a plus, unchanged plain. */
export function showHistory(title: string, versions: Version[]) {
  void openModal((close) => (
    <ModalFrame title={title} onClose={() => close()}>
      <History versions={versions} />
    </ModalFrame>
  ));
}
