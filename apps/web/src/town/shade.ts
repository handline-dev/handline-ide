// Turns scene3d's buffers into pixel art: light in three steps (shade a little cool), a dark 1-px outline on the near side of every edge
// between objects, and a light rim where a face turns up from the one below it. The result is top-down RGBA plus, per pixel, the object
// id and depth that people.ts places the people by.
import type { Frame } from '#src/town/scene3d.ts';

export type Picture = { w: number; h: number; rgba: Uint8ClampedArray<ArrayBuffer>; id: Int32Array; depth: Float32Array };

const SHADE_TINT = [0.93, 0.95, 1.05];
const INK = [0.04, 0.03, 0.06];
const FLAT = new Set(['floor', 'baseboard']);

const unpack = (p: number): [number, number, number] => [
  (Math.floor(p / 65536) / 255) * 2 - 1,
  ((Math.floor(p / 256) % 256) / 255) * 2 - 1,
  ((p % 256) / 255) * 2 - 1,
];

export function shade(f: Frame): Picture {
  const { w, h } = f;
  const n = w * h;
  const id = new Int32Array(n);
  const depth = new Float32Array(n).fill(1e9);
  const normal = new Float32Array(n * 3);
  const col = new Float32Array(n * 3);
  // GL rows run bottom-up; the picture's run top-down.
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const i = y * w + x;
      const g = ((h - 1 - y) * w + x) * 4;
      const o = Math.round(f.info[g]);
      if (!o) continue;
      id[i] = o;
      depth[i] = f.info[g + 1];
      normal.set(unpack(f.info[g + 2]), i * 3);
      const lam = f.light[g] / 255;
      const glow = f.base[g + 3] > 127;
      const step = glow ? 1 : lam > 0.55 ? 1 : lam > 0.2 ? 0.91 : 0.82;
      for (let c = 0; c < 3; c++) col[i * 3 + c] = (f.base[g + c] / 255) * step * (step < 1 ? SHADE_TINT[c] : 1);
    }
  }
  const out = col.slice();
  // Floor-like surfaces never take a line; every other object takes one wherever it meets floor, the background or anything farther.
  const flat = new Uint8Array(n);
  for (let i = 0; i < n; i++) flat[i] = !id[i] || FLAT.has(f.meta[id[i]]?.kind ?? '') ? 1 : 0;
  for (let i = 0; i < n; i++) if (id[i]) edgeOf(i, { w, h, id, depth, normal, col, out, flat }, f.meta[id[i]]?.kind);
  const rgba = new Uint8ClampedArray(n * 4);
  for (let i = 0; i < n; i++) {
    if (!id[i]) continue;
    rgba.set([out[i * 3] * 255, out[i * 3 + 1] * 255, out[i * 3 + 2] * 255, 255], i * 4);
  }
  return { w, h, rgba, id, depth };
}

type Buffers = {
  w: number;
  h: number;
  id: Int32Array;
  depth: Float32Array;
  normal: Float32Array;
  col: Float32Array;
  out: Float32Array;
  flat: Uint8Array;
};
const SIDES = [
  [1, 0],
  [-1, 0],
  [0, 1],
  [0, -1],
];

/** Pixel i against its four neighbours: inked where it meets the floor or a farther object (so between two objects exactly the nearer
 *  one takes the line, and none is lost), rimmed where its own face turns up from the next one. */
function edgeOf(i: number, b: Buffers, kind: string | undefined) {
  const { w, h, id, depth, normal, col, out, flat } = b;
  const [x, y] = [i % w, Math.floor(i / w)];
  const outlined = !flat[i];
  let rim = false;
  for (const [dx, dy] of SIDES) {
    const [x2, y2] = [x + dx, y + dy];
    const j = y2 * w + x2;
    const inside = x2 >= 0 && x2 < w && y2 >= 0 && y2 < h;
    const [o2, d2, behind] = inside ? [id[j], depth[j], flat[j]] : [0, 1e9, 1];
    if (o2 !== id[i] && outlined && (behind || d2 >= depth[i])) {
      // The dark line wins over any rim, so a line never breaks where a top face meets its side.
      for (let c = 0; c < 3; c++) out[i * 3 + c] = col[i * 3 + c] * 0.42 + INK[c];
      return;
    }
    if (o2 !== id[i] || kind === 'floor') continue;
    const dot = normal[i * 3] * normal[j * 3] + normal[i * 3 + 1] * normal[j * 3 + 1] + normal[i * 3 + 2] * normal[j * 3 + 2];
    rim ||= dot < 0.7 && normal[i * 3 + 1] > normal[j * 3 + 1];
  }
  if (rim) for (let c = 0; c < 3; c++) out[i * 3 + c] = Math.min(col[i * 3 + c] * 1.18, 1);
}
