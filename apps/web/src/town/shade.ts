// Turns scene3d's buffers into pixel art: light in three steps (shade a little cool), a dark 1-px outline on the near side of every edge
// between objects, and a light rim where a face turns up from the one below it. The result is top-down RGBA plus, per pixel, the object
// id and depth that people.ts places the people by.
import type { Frame } from '#src/town/scene3d.ts';

export type Picture = { w: number; h: number; rgba: Uint8ClampedArray<ArrayBuffer>; id: Int32Array; depth: Float32Array };

const SHADE_TINT = [0.93, 0.95, 1.05];
const INK = [0.04, 0.03, 0.06];
const FLAT = new Set(['floor', 'baseboard', 'road', 'walk', 'grass', 'lane']);

export function shade(f: Frame): Picture {
  const { w, h } = f;
  const n = w * h;
  const id = new Int32Array(n);
  const depth = new Float32Array(n).fill(1e9);
  const normal = new Float32Array(n * 3);
  const col = new Float32Array(n * 3);
  // GL rows run bottom-up; the picture's run top-down. Typed arrays and no allocation a pixel: a city is millions of them.
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const i = y * w + x;
      const g = ((h - 1 - y) * w + x) * 4;
      const o = Math.round(f.info[g]);
      if (!o) continue;
      id[i] = o;
      depth[i] = f.info[g + 1];
      const p = f.info[g + 2];
      normal[i * 3] = (Math.floor(p / 65536) / 255) * 2 - 1;
      normal[i * 3 + 1] = ((Math.floor(p / 256) % 256) / 255) * 2 - 1;
      normal[i * 3 + 2] = ((p % 256) / 255) * 2 - 1;
      const lam = f.light[g] / 255;
      const glow = f.base[g + 3] > 127;
      const step = glow ? 1 : lam > 0.55 ? 1 : lam > 0.2 ? 0.91 : 0.82;
      for (let c = 0; c < 3; c++) col[i * 3 + c] = (f.base[g + c] / 255) * step * (step < 1 ? SHADE_TINT[c] : 1);
    }
  }
  const out = col.slice();
  // Floor-like surfaces never take a line; every other object takes one wherever it meets floor, the background or anything farther.
  const flatOf = new Uint8Array(f.meta.length + 1);
  const floorOf = new Uint8Array(f.meta.length + 1);
  flatOf[0] = 1;
  f.meta.forEach((m, k) => {
    flatOf[k] = k === 0 || FLAT.has(m.kind) ? 1 : 0;
    floorOf[k] = m.kind === 'floor' ? 1 : 0;
  });
  for (let i = 0; i < n; i++) if (id[i]) edgeOf(i, w, h, id, depth, normal, col, out, flatOf, floorOf[id[i]] === 1);
  const rgba = new Uint8ClampedArray(n * 4);
  for (let i = 0; i < n; i++) {
    if (!id[i]) continue;
    rgba[i * 4] = out[i * 3] * 255;
    rgba[i * 4 + 1] = out[i * 3 + 1] * 255;
    rgba[i * 4 + 2] = out[i * 3 + 2] * 255;
    rgba[i * 4 + 3] = 255;
  }
  return { w, h, rgba, id, depth };
}

const DX = [1, -1, 0, 0];
const DY = [0, 0, 1, -1];

/** Pixel i against its four neighbours: inked where it meets the floor or a farther object (so between two objects exactly the nearer
 *  one takes the line, and none is lost), rimmed where its own face turns up from the next one. */
function edgeOf(
  i: number,
  w: number,
  h: number,
  id: Int32Array,
  depth: Float32Array,
  normal: Float32Array,
  col: Float32Array,
  out: Float32Array,
  flatOf: Uint8Array,
  floor: boolean,
) {
  const x = i % w;
  const y = (i - x) / w;
  const me = id[i];
  const outlined = !flatOf[me];
  let rim = false;
  for (let k = 0; k < 4; k++) {
    const x2 = x + DX[k];
    const y2 = y + DY[k];
    const inside = x2 >= 0 && x2 < w && y2 >= 0 && y2 < h;
    const j = y2 * w + x2;
    const o2 = inside ? id[j] : 0;
    if (o2 !== me && outlined && (!inside || flatOf[o2] || depth[j] >= depth[i])) {
      // The dark line wins over any rim, so a line never breaks where a top face meets its side.
      for (let c = 0; c < 3; c++) out[i * 3 + c] = col[i * 3 + c] * 0.42 + INK[c];
      return;
    }
    if (o2 !== me || floor) continue;
    const dot = normal[i * 3] * normal[j * 3] + normal[i * 3 + 1] * normal[j * 3 + 1] + normal[i * 3 + 2] * normal[j * 3 + 2];
    rim ||= dot < 0.7 && normal[i * 3 + 1] > normal[j * 3 + 1];
  }
  if (rim) for (let c = 0; c < 3; c++) out[i * 3 + c] = Math.min(col[i * 3 + c] * 1.18, 1);
}
