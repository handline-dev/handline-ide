// What turned one text into another, as CodeMirror changes: line by line, each changed run its own change, so a change that arrives
// from elsewhere can be laid over local edits and keep them (one change over the whole span would swallow what was typed inside it).
import { ChangeSet, type ChangeSpec } from '@codemirror/state';

/** The largest middle (lines × lines) compared exactly; beyond it the middle is one change. */
const MAX_CELLS = 4_000_000;

export function diffChanges(a: string, b: string): ChangeSet {
  const x = a.split('\n');
  const y = b.split('\n');
  // The lines alike at both ends are left out of the comparison.
  let pre = 0;
  while (pre < x.length && pre < y.length && x[pre] === y[pre]) pre++;
  let suf = 0;
  while (suf < x.length - pre && suf < y.length - pre && x[x.length - 1 - suf] === y[y.length - 1 - suf]) suf++;
  const xs = x.slice(pre, x.length - suf);
  const ys = y.slice(pre, y.length - suf);
  // Offsets of each line's start in a.
  const start: number[] = [];
  let at = 0;
  for (const l of x) {
    start.push(at);
    at += l.length + 1;
  }
  const off = (i: number) => (i < x.length ? start[i] : a.length);
  const changes: ChangeSpec[] = [];
  const replace = (i0: number, i1: number, j0: number, j1: number) => {
    // Lines i0..i1 of a (absolute) become ys j0..j1.
    const ins = ys.slice(j0, j1);
    if (i0 === i1 && !ins.length) return;
    if (i0 === i1) {
      // Pure insert before line i0.
      if (i0 < x.length) changes.push({ from: off(i0), insert: ins.join('\n') + '\n' });
      else changes.push({ from: a.length, insert: '\n' + ins.join('\n') });
      return;
    }
    if (!ins.length) {
      // Pure delete of whole lines, with the line break that ends them (or begins them at the end of the text).
      const from = i1 < x.length ? off(i0) : Math.max(0, off(i0) - 1);
      const to = i1 < x.length ? off(i1) : a.length;
      changes.push({ from, to });
      return;
    }
    const to = i1 < x.length ? off(i1) - 1 : a.length;
    changes.push({ from: off(i0), to, insert: ins.join('\n') });
  };
  if (xs.length * ys.length > MAX_CELLS) {
    replace(pre, x.length - suf, 0, ys.length);
    return ChangeSet.of(changes, a.length);
  }
  // Longest common subsequence of the middle lines; the runs between its matches are the changes.
  const n = xs.length;
  const m = ys.length;
  const w = m + 1;
  const L = new Uint32Array((n + 1) * w);
  for (let i = n - 1; i >= 0; i--)
    for (let j = m - 1; j >= 0; j--)
      L[i * w + j] = xs[i] === ys[j] ? L[(i + 1) * w + j + 1] + 1 : Math.max(L[(i + 1) * w + j], L[i * w + j + 1]);
  let i = 0;
  let j = 0;
  let ri = 0;
  let rj = 0;
  while (i < n && j < m) {
    if (xs[i] === ys[j]) {
      replace(pre + ri, pre + i, rj, j);
      i++;
      j++;
      ri = i;
      rj = j;
    } else if (L[(i + 1) * w + j] >= L[i * w + j + 1]) i++;
    else j++;
  }
  replace(pre + ri, pre + n, rj, m);
  return ChangeSet.of(changes, a.length);
}
