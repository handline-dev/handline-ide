// An account's usage over time as SVG path data: percent used (100 - left) against time, a gap where no sample was read. Pure.

type Point = { at: string; left: number | null };

/** The path of `points` in a box `w` by `h`, from `from` to `to` (ms): 0% used at the bottom, 100% at the top; a null breaks it. */
export function usedPath(points: readonly Point[], from: number, to: number, w: number, h: number): string {
  const span = Math.max(1, to - from);
  let d = '';
  let pen = false;
  for (const p of points) {
    if (p.left === null) {
      pen = false;
      continue;
    }
    const x = ((Date.parse(p.at) - from) / span) * w;
    const y = (Math.min(100, Math.max(0, p.left)) / 100) * h;
    d += `${pen ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`;
    pen = true;
  }
  return d;
}
