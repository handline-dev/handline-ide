// The office's furniture, built from boxes and cylinders as the city is (street.ts). A piece stands with its left front corner at (x, z)
// and reaches right along x and back along -z, facing the camera; a plant is centred on (x, z). The boxes of one piece are one part,
// outlined only around the whole, and a piece's `tag` names it for the pixels that show it.
import type { Put } from '#src/town/room.ts';

type RGB = [number, number, number];

export const DESK_TOP = 0.384;
const DESK: RGB = [236, 236, 232];
const STEEL: RGB = [176, 182, 192];
const INK: RGB = [52, 56, 70];
const CUSHION: RGB = [66, 70, 90];
const SCREEN_LIT: RGB = [150, 190, 235];
const SCREEN_DARK: RGB = [58, 62, 78];
const POT: RGB = [240, 238, 232];
const SOIL: RGB = [110, 78, 56];
const LEAVES: RGB[] = [
  [72, 140, 64],
  [88, 162, 72],
  [108, 180, 86],
];
const WOOD: RGB = [196, 132, 80];
const BOOKS: RGB[] = [
  [70, 110, 190],
  [240, 236, 224],
  [230, 170, 60],
  [200, 80, 80],
  [90, 150, 100],
  [130, 90, 160],
];

/** A desk 0.73 m wide and 0.39 m deep: a white top on four square steel legs, a panel across the back between them. */
export function desk(put: Put, x: number, z: number, tag: string) {
  const part = `desk:${x},${z}`;
  const [x0, x1, z0, z1] = [x, x + 0.734, z - 0.39, z];
  put([x0, DESK_TOP - 0.03, z0], [x1, DESK_TOP, z1], 'thing', { part, tag, colour: DESK, solid: true });
  for (const lx of [x0 + 0.02, x1 - 0.08])
    for (const lz of [z0 + 0.02, z1 - 0.08]) put([lx, 0, lz], [lx + 0.06, DESK_TOP - 0.03, lz + 0.06], 'thing', { part, colour: STEEL });
  put([x0 + 0.05, 0.12, z0 + 0.03], [x1 - 0.05, DESK_TOP - 0.03, z0 + 0.05], 'thing', { part, colour: STEEL });
}

/** A desk chair 0.3 m across: a cross of legs on castors under a post, a seat, a back and two arms. Its back is toward the camera, the
 *  sitter facing the desk, or `turned` round against the desk, the sitter facing the camera. */
export function chair(put: Put, x: number, z: number, tag: string, turned = false) {
  const part = `chair:${x},${z}`;
  const [cx, cz] = [x + 0.142, z - 0.13];
  // Turned, what stands at a depth `a` before the seat's middle stands as far behind it.
  const across = (a: number, b: number): [number, number] => (turned ? [2 * (z - 0.145) - b, 2 * (z - 0.145) - a] : [a, b]);
  put([cx - 0.13, 0.02, cz - 0.02], [cx + 0.13, 0.05, cz + 0.02], 'thing', { part, tag, colour: INK });
  put([cx - 0.02, 0.02, cz - 0.13], [cx + 0.02, 0.05, cz + 0.13], 'thing', { part, colour: INK });
  for (const [dx, dz] of [
    [-0.12, 0],
    [0.12, 0],
    [0, -0.12],
    [0, 0.12],
  ])
    put([cx + dx - 0.02, 0, cz + dz - 0.02], [cx + dx + 0.02, 0.03, cz + dz + 0.02], 'thing', { part, colour: INK, round: 'x' });
  put([cx - 0.022, 0.05, cz - 0.022], [cx + 0.022, 0.2, cz + 0.022], 'thing', { part, colour: STEEL, round: 'y' });
  put([x, 0.2, z - 0.27], [x + 0.285, 0.25, z - 0.02], 'thing', { part, colour: CUSHION, solid: true });
  const back = across(z - 0.06, z);
  put([x + 0.02, 0.22, back[0]], [x + 0.265, 0.6, back[1]], 'thing', { part, colour: CUSHION });
  for (const ax of [x - 0.01, x + 0.265]) {
    const [arm, post] = [across(z - 0.2, z - 0.06), across(z - 0.15, z - 0.12)];
    put([ax, 0.25, arm[0]], [ax + 0.03, 0.29, arm[1]], 'thing', { part, colour: INK });
    put([ax + 0.005, 0.2, post[0]], [ax + 0.025, 0.25, post[1]], 'thing', { part, colour: INK });
  }
}

/** A screen on a desk at height y, facing the camera: a foot, a neck and the panel, its picture lit (with lines of text) while `lit`. */
export function screen(put: Put, x: number, z: number, y: number, lit: boolean) {
  const part = `screen:${x},${z}`;
  const mid = x + 0.197;
  put([mid - 0.07, y, z - 0.1], [mid + 0.07, y + 0.01, z - 0.02], 'thing', { part, colour: INK });
  put([mid - 0.018, y, z - 0.09], [mid + 0.018, y + 0.1, z - 0.06], 'thing', { part, colour: INK });
  put([x, y + 0.08, z - 0.06], [x + 0.393, y + 0.294, z - 0.03], 'thing', { part, colour: INK });
  if (lit) put([x + 0.02, y + 0.1, z - 0.03], [x + 0.373, y + 0.274, z - 0.022], 'lit', { part, colour: SCREEN_LIT, pattern: 3 });
  else put([x + 0.02, y + 0.1, z - 0.03], [x + 0.373, y + 0.274, z - 0.022], 'thing', { part, colour: SCREEN_DARK });
}

/** A keyboard and, `mouse` metres to its right, a mouse, on a desk at height y. */
export function keyboard(put: Put, x: number, z: number, y: number, mouse?: number) {
  const part = `keys:${x},${z}`;
  put([x, y, z - 0.118], [x + 0.282, y + 0.014, z], 'thing', { part, colour: [60, 64, 78] });
  put([x + 0.015, y + 0.014, z - 0.105], [x + 0.267, y + 0.026, z - 0.015], 'thing', { part, colour: [96, 100, 116] });
  if (mouse !== undefined)
    put([x + mouse - 0.025, y, z - 0.085], [x + mouse + 0.025, y + 0.024, z], 'thing', { colour: [60, 64, 78], round: 'z' });
}

/** A plant in a round white pot centred on (x, z) at height y, `size` metres tall: a short trunk under a round crown of leaf clumps, the
 *  lower ones darker. */
export function plant(put: Put, x: number, z: number, y: number, size: number) {
  const part = `plant:${x},${z},${y}`;
  const r = size * 0.14;
  const pot = size * 0.32;
  put([x - r, y, z - r], [x + r, y + pot, z + r], 'thing', { part, colour: POT, round: 'y', solid: y === 0 });
  put([x - r * 0.85, y + pot, z - r * 0.85], [x + r * 0.85, y + pot + 0.005, z + r * 0.85], 'thing', { part, colour: SOIL, round: 'y' });
  const trunk = size * 0.025;
  put([x - trunk, y + pot, z - trunk], [x + trunk, y + size * 0.6, z + trunk], 'thing', { part, colour: BARK, round: 'y' });
  // Clumps as (across, back, up, radius, height), in crowns: a ring low, a smaller ring over it, one on top.
  const crown = size * 0.22;
  const mid = y + size * 0.62;
  for (const [dx, dz, dy, cr, ch, shade] of CLUMPS) {
    const [cx, cz, cy, rr] = [x + dx * crown, z + dz * crown, mid + dy * crown, cr * crown];
    put([cx - rr, cy - ch * crown, cz - rr], [cx + rr, cy + ch * crown, cz + rr], 'thing', { part, colour: LEAVES[shade], round: 'y' });
  }
}

const BARK: RGB = [120, 86, 60];
const CLUMPS: [number, number, number, number, number, 0 | 1 | 2][] = [
  [0, 0, 0, 0.95, 0.5, 0],
  [-0.55, 0.15, -0.1, 0.55, 0.38, 0],
  [0.55, 0.2, -0.05, 0.55, 0.38, 0],
  [0.05, 0.5, -0.15, 0.55, 0.36, 1],
  [-0.3, -0.35, 0.35, 0.6, 0.4, 1],
  [0.35, -0.2, 0.4, 0.55, 0.38, 1],
  [-0.2, 0.3, 0.5, 0.5, 0.34, 2],
  [0.25, 0.35, 0.55, 0.45, 0.3, 2],
  [0, 0, 0.9, 0.45, 0.3, 2],
];

/** A two-seat sofa 0.98 m wide facing the camera: a base on short wooden feet, two seat cushions with a seam between, a slim back and arms
 *  rolled over at the top. */
export function sofa(put: Put, x: number, z: number) {
  const part = `sofa:${x},${z}`;
  const fabric: RGB = [104, 160, 150];
  const roll: RGB = [116, 174, 164];
  put([x, 0.04, z - 0.41], [x + 0.98, 0.2, z], 'thing', { part, colour: fabric, solid: true });
  for (const fx of [x + 0.03, x + 0.91])
    for (const fz of [z - 0.38, z - 0.07]) put([fx, 0, fz], [fx + 0.04, 0.04, fz + 0.04], 'thing', { part, colour: [120, 80, 50] });
  put([x, 0.2, z - 0.41], [x + 0.98, 0.4, z - 0.35], 'thing', { part, colour: fabric });
  put([x, 0.38, z - 0.41], [x + 0.98, 0.42, z - 0.35], 'thing', { part, colour: roll, round: 'x' });
  for (const ax of [x, x + 0.86]) {
    put([ax, 0.2, z - 0.41], [ax + 0.12, 0.3, z], 'thing', { part, colour: fabric });
    put([ax, 0.24, z - 0.41], [ax + 0.12, 0.36, z], 'thing', { part, colour: roll, round: 'z' });
  }
  for (const sx of [x + 0.12, x + 0.495]) put([sx, 0.2, z - 0.35], [sx + 0.36, 0.28, z - 0.01], 'thing', { part, colour: roll });
}

/** A low coffee table, its top 0.66 by 0.4 m, on four legs, with a shelf under. */
export function coffeeTable(put: Put, x: number, z: number) {
  const part = `table:${x},${z}`;
  const [x0, x1, z0, z1] = [x, x + 0.66, z - 0.4, z];
  put([x0, 0.2, z0], [x1, 0.23, z1], 'thing', { part, colour: [200, 142, 88], solid: true });
  for (const lx of [x0 + 0.02, x1 - 0.06])
    for (const lz of [z0 + 0.02, z1 - 0.06]) put([lx, 0, lz], [lx + 0.04, 0.2, lz + 0.04], 'thing', { part, colour: [150, 100, 62] });
  put([x0 + 0.05, 0.06, z0 + 0.05], [x1 - 0.05, 0.08, z1 - 0.05], 'thing', { part, colour: [176, 122, 76] });
}

/** A rug 1.57 by 0.92 m lying flat, a darker band round its middle. */
export function rug(put: Put, x: number, z: number) {
  put([x, 0, z - 0.92], [x + 1.57, 0.006, z], 'rug');
  put([x + 0.12, 0.006, z - 0.8], [x + 1.45, 0.008, z - 0.12], 'rug', { colour: [226, 210, 184] });
  put([x + 0.2, 0.008, z - 0.72], [x + 1.37, 0.01, z - 0.2], 'rug');
}

/** An open bookcase 0.4 m wide and 0.88 m tall against the wall, three shelves of books of every colour and height. */
export function bookcase(put: Put, x: number, z: number) {
  const part = `bookcase:${x},${z}`;
  const [x1, z0, t] = [x + 0.4, z - 0.25, 0.03];
  put([x, 0, z0], [x1, t, z], 'thing', { part, colour: WOOD, solid: true });
  put([x, t, z0], [x1, 0.88, z0 + 0.02], 'thing', { part, colour: [170, 110, 64] });
  for (const sx of [x, x1 - t]) put([sx, t, z0], [sx + t, 0.88, z], 'thing', { part, colour: WOOD });
  for (const y of [0.2, 0.42, 0.64, 0.85]) put([x + t, y, z0], [x1 - t, y + t, z], 'thing', { part, colour: WOOD });
  [0.23, 0.45, 0.67].forEach((y, shelf) => {
    let bx = x + t + 0.01;
    for (let k = 0; ; k++) {
      const n = shelf * 7 + k;
      const w = 0.03 + ((n * 7) % 3) * 0.008;
      if (bx + w > x1 - t) break;
      const h = 0.11 + ((n * 5) % 4) * 0.015;
      if ((n * 3) % 7 !== 6) put([bx, y, z0 + 0.03], [bx + w, y + h, z - 0.03], 'thing', { part, colour: BOOKS[(n * 5) % BOOKS.length] });
      bx += w + 0.004;
    }
  });
}

/** A kitchen cabinet 0.43 m wide and 0.45 m tall: a body, a darker counter over it, two doors with handles. */
export function cabinet(put: Put, x: number, z: number) {
  const part = `cabinet:${x},${z}`;
  put([x, 0, z - 0.45], [x + 0.43, 0.42, z - 0.02], 'thing', { part, colour: [236, 224, 206], solid: true });
  put([x - 0.01, 0.42, z - 0.46], [x + 0.44, 0.45, z + 0.01], 'thing', { part, colour: [196, 168, 132] });
  for (const dx of [x + 0.02, x + 0.22]) {
    put([dx, 0.04, z - 0.02], [dx + 0.19, 0.4, z], 'thing', { part, colour: [244, 236, 222] });
    const hx = dx === x + 0.02 ? dx + 0.15 : dx + 0.02;
    put([hx, 0.26, z], [hx + 0.02, 0.34, z + 0.015], 'thing', { part, colour: [150, 150, 160] });
  }
}

/** A red espresso machine at height y: its body at the back, a steel top, the head over a drip tray and a white cup in front of it. */
export function coffeeMachine(put: Put, x: number, z: number, y: number) {
  const part = `coffee:${x},${z}`;
  const red: RGB = [196, 64, 56];
  put([x, y, z - 0.24], [x + 0.19, y + 0.17, z - 0.1], 'thing', { part, colour: red });
  put([x, y + 0.17, z - 0.24], [x + 0.19, y + 0.19, z - 0.1], 'thing', { part, colour: STEEL });
  put([x + 0.03, y + 0.11, z - 0.1], [x + 0.16, y + 0.15, z - 0.05], 'thing', { part, colour: red });
  put([x + 0.01, y, z - 0.1], [x + 0.18, y + 0.02, z], 'thing', { part, colour: STEEL });
  put([x + 0.065, y + 0.02, z - 0.06], [x + 0.125, y + 0.08, z], 'thing', { colour: [244, 242, 236], round: 'y' });
}
