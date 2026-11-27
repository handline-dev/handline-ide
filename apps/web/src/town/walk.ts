// Where the founder can walk, in an office (metres) or the city (tiles): the ground as a grid of small cells, blocked where something
// stands (with room for the body), the shortest way to a clicked spot, and a step for held keys that slides along what it meets.

/** A footprint on the ground. */
export type Rect = { x0: number; z0: number; x1: number; z1: number };
export type Floor = { cols: number; rows: number; cell: number; free: Uint8Array };
export type Spot = [number, number];

/** `cell`: the grid's step; `body`: how far the body keeps from anything; `top`: what is blocked along the top edge (an office's
 *  baseboard). The defaults are an office's, in metres. */
export type Grid = { cell: number; body: number; top: number };
const OFFICE: Grid = { cell: 0.05, body: 0.12, top: 0.12 };

export function floorOf(w: number, d: number, solid: readonly Rect[], { cell, body, top }: Grid = OFFICE): Floor {
  const [cols, rows] = [Math.ceil(w / cell), Math.ceil(d / cell)];
  const free = new Uint8Array(cols * rows);
  for (let r = 0; r < rows; r++) {
    const z = (r + 0.5) * cell;
    if (z <= top + body || z >= d - body) continue;
    for (let c = 0; c < cols; c++) {
      const x = (c + 0.5) * cell;
      free[r * cols + c] = x > body && x < w - body ? 1 : 0;
    }
  }
  // Each footprint, grown by the body, blocks the cells whose middles it covers.
  for (const s of solid) {
    const [c0, c1] = [Math.max(0, Math.ceil((s.x0 - body) / cell - 0.5)), Math.min(cols - 1, Math.floor((s.x1 + body) / cell - 0.5))];
    const [r0, r1] = [Math.max(0, Math.ceil((s.z0 - body) / cell - 0.5)), Math.min(rows - 1, Math.floor((s.z1 + body) / cell - 0.5))];
    for (let r = r0; r <= r1; r++) free.fill(0, r * cols + c0, r * cols + c1 + 1);
  }
  return { cols, rows, cell, free };
}

const cellOf = (f: Floor, [x, z]: Spot): [number, number] => [
  Math.min(f.cols - 1, Math.max(0, Math.floor(x / f.cell))),
  Math.min(f.rows - 1, Math.max(0, Math.floor(z / f.cell))),
];
const centreOf = (f: Floor, c: number, r: number): Spot => [(c + 0.5) * f.cell, (r + 0.5) * f.cell];
export const freeAt = (f: Floor, at: Spot) => {
  const [c, r] = cellOf(f, at);
  return f.free[r * f.cols + c] === 1;
};

/** The free cell nearest to `at`, searched outwards ring by ring; `at` itself when free. */
export function nearestFree(f: Floor, at: Spot): Spot | null {
  const [c0, r0] = cellOf(f, at);
  for (let k = 0; k < Math.max(f.cols, f.rows); k++) {
    let best: [number, number, number] | null = null;
    for (let r = r0 - k; r <= r0 + k; r++) {
      for (let c = c0 - k; c <= c0 + k; c++) {
        if (Math.max(Math.abs(r - r0), Math.abs(c - c0)) !== k || c < 0 || r < 0 || c >= f.cols || r >= f.rows) continue;
        if (!f.free[r * f.cols + c]) continue;
        const d = (c - c0) ** 2 + (r - r0) ** 2;
        if (!best || d < best[2]) best = [c, r, d];
      }
    }
    if (best) return k === 0 ? at : centreOf(f, best[0], best[1]);
  }
  return null;
}

/** Whether a straight walk from a to b stays on free cells. */
function clear(f: Floor, a: Spot, b: Spot): boolean {
  const n = Math.ceil(Math.hypot(b[0] - a[0], b[1] - a[1]) / (f.cell / 2));
  for (let i = 1; i <= n; i++) if (!freeAt(f, [a[0] + ((b[0] - a[0]) * i) / n, a[1] + ((b[1] - a[1]) * i) / n])) return false;
  return true;
}

const STEPS = [
  [1, 0],
  [-1, 0],
  [0, 1],
  [0, -1],
  [1, 1],
  [1, -1],
  [-1, 1],
  [-1, -1],
] as const;

/** A binary min-heap of cells by their guessed cost. */
function heap() {
  const ids: number[] = [];
  const keys: number[] = [];
  const swap = (a: number, b: number) => {
    [ids[a], ids[b]] = [ids[b], ids[a]];
    [keys[a], keys[b]] = [keys[b], keys[a]];
  };
  return {
    size: () => ids.length,
    push: (id: number, key: number) => {
      ids.push(id);
      keys.push(key);
      for (let i = ids.length - 1; i > 0;) {
        const up = (i - 1) >> 1;
        if (keys[up] <= keys[i]) break;
        swap(i, up);
        i = up;
      }
    },
    pop: (): number => {
      const top = ids[0];
      const last = ids.length - 1;
      swap(0, last);
      ids.pop();
      keys.pop();
      for (let i = 0; ;) {
        const [l, r] = [2 * i + 1, 2 * i + 2];
        let m = i;
        if (l < ids.length && keys[l] < keys[m]) m = l;
        if (r < ids.length && keys[r] < keys[m]) m = r;
        if (m === i) break;
        swap(i, m);
        i = m;
      }
      return top;
    },
  };
}

/** The turns of the shortest walk from `from` to `to` (A* over the cells, then cut to the corners it cannot see past); null when none. */
export function pathTo(f: Floor, from: Spot, to: Spot): Spot[] | null {
  const goal = nearestFree(f, to);
  if (!goal) return null;
  const [sc, sr] = cellOf(f, from);
  const [gc, gr] = cellOf(f, goal);
  const n = f.cols * f.rows;
  const cost = new Float32Array(n).fill(Infinity);
  const prev = new Int32Array(n).fill(-1);
  const done = new Uint8Array(n);
  const target = gr * f.cols + gc;
  const guess = (i: number) => cost[i] + Math.hypot((i % f.cols) - gc, Math.floor(i / f.cols) - gr);
  const open = heap();
  cost[sr * f.cols + sc] = 0;
  open.push(sr * f.cols + sc, guess(sr * f.cols + sc));
  while (open.size()) {
    const i = open.pop();
    if (done[i]) continue; // a cell is pushed again when a cheaper way to it turns up; the older entry is stale
    done[i] = 1;
    if (i === target) break;
    const [c, r] = [i % f.cols, Math.floor(i / f.cols)];
    for (const [dc, dr] of STEPS) {
      const [c2, r2] = [c + dc, r + dr];
      if (c2 < 0 || r2 < 0 || c2 >= f.cols || r2 >= f.rows) continue;
      const j = r2 * f.cols + c2;
      // A diagonal step needs both cells beside it free, so it never cuts a corner.
      if (done[j] || !f.free[j] || (dc && dr && (!f.free[r * f.cols + c2] || !f.free[r2 * f.cols + c]))) continue;
      const g = cost[i] + (dc && dr ? Math.SQRT2 : 1);
      if (g >= cost[j]) continue;
      cost[j] = g;
      prev[j] = i;
      open.push(j, guess(j));
    }
  }
  if (cost[target] === Infinity) return null;
  const cells: Spot[] = [];
  for (let i = target; i !== -1; i = prev[i]) cells.unshift(centreOf(f, i % f.cols, Math.floor(i / f.cols)));
  cells[cells.length - 1] = goal;
  const turns: Spot[] = [];
  let at = from;
  for (let i = 1; i < cells.length; i++) {
    if (i === cells.length - 1 || !clear(f, at, cells[i + 1])) {
      turns.push(cells[i]);
      at = cells[i];
    }
  }
  return turns;
}

/** How many times a walk's speed a run is. */
const RUN = 2;
/** How far held keys take the founder in `dt` seconds at a walk of `speed`, twice as far running. */
export const reach = (speed: number, dt: number, running: boolean) => speed * dt * (running ? RUN : 1);

/** One step of `dist` metres along (dx, dz) from `at`; blocked one way, it slides along the other. */
export function step(f: Floor, at: Spot, [dx, dz]: Spot, dist: number): Spot {
  const len = Math.hypot(dx, dz) || 1;
  const [mx, mz] = [(dx / len) * dist, (dz / len) * dist];
  for (const next of [
    [at[0] + mx, at[1] + mz],
    [at[0] + mx, at[1]],
    [at[0], at[1] + mz],
  ] as Spot[]) {
    if ((next[0] !== at[0] || next[1] !== at[1]) && freeAt(f, next)) return next;
  }
  return at;
}
