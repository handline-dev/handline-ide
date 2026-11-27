// The HQ's dotted person (Cast.tsx): a figure's outline on a coarse grid of its own, every other cell of it, so the dots stay even
// however the sheet's figure was scaled.

/** The cells of a `step`-pixel grid over an image `w` by `h` (its alpha, row by row) that make a dotted outline: those just outside the
 *  figure, touching it on a side or corner, every other one. A cell is in the figure when at least half its pixels are. */
export function outlineDots(alpha: ArrayLike<number>, w: number, h: number, step: number): [number, number][] {
  const [gw, gh] = [Math.ceil(w / step), Math.ceil(h / step)];
  /** Whether at least half the pixels of cell (gx, gy) are the figure's. */
  const half = (gx: number, gy: number) => {
    let [n, all] = [0, 0];
    for (let y = gy * step; y < Math.min(h, (gy + 1) * step); y++) {
      for (let x = gx * step; x < Math.min(w, (gx + 1) * step); x++, all++) if (alpha[y * w + x] > 127) n++;
    }
    return n * 2 >= all;
  };
  const inside = Uint8Array.from({ length: gw * gh }, (_, i) => (half(i % gw, Math.floor(i / gw)) ? 1 : 0));
  const isIn = (x: number, y: number) => x >= 0 && y >= 0 && x < gw && y < gh && inside[y * gw + x] === 1;
  const dots: [number, number][] = [];
  for (let gy = 0; gy < gh; gy++) {
    for (let gx = 0; gx < gw; gx++) {
      if (isIn(gx, gy) || (gx + gy) % 2) continue;
      // The ring round a figure is a path of side steps, so alternate cells along it alternate in parity.
      const touches = [-1, 0, 1].some((dy) => [-1, 0, 1].some((dx) => isIn(gx + dx, gy + dy)));
      if (touches) dots.push([gx, gy]);
    }
  }
  return dots;
}
