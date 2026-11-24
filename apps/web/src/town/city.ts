// The city's layout and its drawing from Kenney's Roguelike Modern City sheet (public/town/city.png, 16px tiles, CC0), seen from the
// front-top like the sheet's own sample. Streets cross in a grid: a strip of blocks with a road under it, an avenue between blocks. A
// workspace is a building facing the road, as wide as its team needs: a window per agent, lit while it works, the lead first. What the
// buildings leave of a block, and the blocks past them out to the pane's edges, are parks.
import type { House } from '#src/town/model.ts';
import type { Activity } from '#src/layout/agentList.ts';

export const T = 16;
export const SHEET = `${import.meta.env.BASE_URL}town/city.png`;
export const HQ = 'hq';

type Tile = readonly [number, number];

// A strip from the top, in tiles: pavement behind the buildings, the buildings, pavement in front, the road.
const BACK = 1;
const BUILD = 6;
const FRONT = 2;
const ROAD = 3;
const STRIP = BACK + BUILD + FRONT + ROAD;
const LOTS = 16; // a block's width for buildings
const SPAN = ROAD + 1 + LOTS + 1; // an avenue, then a block with pavement each side
const WALL = 3; // two floors of windows over the ground floor
const HQ_WIDTH = 6;

/** A building as wide as its team: two floors of windows between its corner columns. */
export const widthFor = (people: number) => Math.min(7, Math.max(4, Math.ceil(people / 2) + 2));
export const seatsOf = (w: number) => 2 * (w - 2);

export type Lot = { key: string; x: number; y: number; w: number; h: number; seats: Activity[]; empty: boolean; look: number };
type Area = { x: number; y: number; w: number; h: number };
export type City = { w: number; h: number; lots: Lot[]; parks: Area[]; blocks: Area[]; roads: number[]; avenues: number[] };

/** A small number that stays the same for a key, so a building keeps its looks. */
const lookOf = (key: string) => [...key].reduce((n, ch) => (n * 31 + ch.charCodeAt(0)) >>> 0, 7);

/** The buildings into blocks in order, as many to a block as fit. */
function blocksOf(widths: number[]): number[][] {
  const out: number[][] = [[]];
  let used = 0;
  widths.forEach((w, i) => {
    if (used + w > LOTS) {
      out.push([]);
      used = 0;
    }
    out[out.length - 1].push(i);
    used += w;
  });
  return out;
}

const sizeOf = (cols: number, strips: number) => [cols * SPAN + ROAD, strips * STRIP];

/** The HQ, then every workspace, in blocks; as many blocks to a strip as fill a pane of this shape (width over height) at the largest
 *  scale, and parks out to the pane's edges. */
export function cityOf(houses: House[], aspect: number): City {
  const items = [
    { key: HQ, w: HQ_WIDTH, seats: [] as Activity[], empty: false },
    ...houses.map((h) => {
      const seats = (h.lead ? [h.lead, ...h.people.filter((p) => p !== h.lead)] : h.people).map((p) => p.activity);
      return { key: h.key, w: widthFor(seats.length), seats, empty: h.working === 0 };
    }),
  ];
  const groups = blocksOf(items.map((i) => i.w));
  let cols = 1;
  let best = 0;
  for (let c = 1; c <= groups.length; c++) {
    const [w, h] = sizeOf(c, Math.ceil(groups.length / c));
    const s = Math.min(aspect / w, 1 / h);
    if (s > best) [cols, best] = [c, s];
  }
  const [cw, ch] = sizeOf(cols, Math.ceil(groups.length / cols));
  const w = Math.max(cw, Math.round(ch * aspect));
  const h = Math.max(ch, Math.round(cw / aspect));

  const lots: Lot[] = [];
  const parks: Area[] = [];
  const blocks: Area[] = [];
  const across = Math.ceil((w - ROAD) / SPAN);
  for (let s = 0; s * STRIP < h; s++)
    for (let c = 0; c < across; c++) {
      const x0 = c * SPAN + ROAD + 1;
      const y0 = s * STRIP;
      blocks.push({ x: x0 - 1, y: y0, w: LOTS + 2, h: BACK + BUILD + FRONT });
      const group = c < cols ? groups[s * cols + c] : undefined;
      let x = x0;
      for (const i of group ?? []) {
        const it = items[i];
        const look = lookOf(it.key);
        const height = it.key === HQ ? BUILD : WALL + 2 + (look % 2);
        lots.push({ ...it, x, y: y0 + BACK + BUILD - height, h: height, look });
        x += it.w;
      }
      if (x0 + LOTS - x >= 2) parks.push({ x, y: y0 + BACK, w: x0 + LOTS - x, h: BUILD });
    }
  const roads = Array.from({ length: Math.ceil(h / STRIP) }, (_, s) => s * STRIP + BACK + BUILD + FRONT);
  const avenues = Array.from({ length: across + 1 }, (_, c) => c * SPAN);
  return { w, h, lots, parks, blocks, roads, avenues };
}

// ---- the sheet -----------------------------------------------------------------------------------------------------------------------

const ASPHALT: Tile = [11, 19];
const PAVEMENT: Tile[] = [
  [7, 19],
  [7, 20],
  [6, 19],
];
const CURB: Tile = [7, 22]; // pavement with its curb along the bottom, 3px
const CURB_PX = 3;
const LANE: Tile = [9, 19]; // a dash along the middle of a horizontal road
const LANE_UP: Tile = [9, 20]; // and of an avenue, from the top
const ZEBRA_ACROSS_ROAD: Tile = [9, 22];
const ZEBRA_ACROSS_AVENUE: Tile = [12, 22];
const GRASS: Tile[] = [
  [0, 24],
  [1, 24],
];
const POTTED: Tile[] = [31, 32, 33, 34, 35, 36].map((c) => [c, 12] as const);
const BUSHES: Tile[] = [31, 32, 33, 34, 35, 36].map((c) => [c, 13] as const);
const TREE_TOP = 10; // a tall tree: rows 10 and 11 of columns 31-36
const LAMP = 8; // a street lamp, rows 16-18
const BENCH: Tile = [17, 16];
const CARTS = 34; // a vendor's cart under a parasol, rows 14-15 of columns 34-36
const BINS: Tile[] = [
  [15, 13],
  [16, 13],
  [15, 14],
];
const ROOF_UNITS: Tile[] = [
  [25, 14],
  [26, 14],
  [27, 14],
  [29, 14],
];

// Buildings. Roofs are nine-slices from a 2x2 corner set, 8 columns apart by colour; walls three-slices 4 columns apart by material.
const STYLES = [
  { roof: 0, wall: 0, window: 0, door: [20, 26] as Tile }, // red roof on red brick, a brown door
  { roof: 8, wall: 4, window: 1, door: [23, 26] as Tile }, // grey on grey brick, a green door
  { roof: 24, wall: 8, window: 0, door: [26, 26] as Tile }, // tan on sandstone, an orange door
  { roof: 16, wall: 0, window: 1, door: [21, 26] as Tile }, // light grey on red brick
];
const WALL_ROWS = [5, 7, 8];
const WINDOWS: Tile[] = [
  [26, 16],
  [26, 19],
];
const GLASS_ROWS = [9, 10, 11, 12]; // the HQ's curtain wall, columns 13-15
const GLASS_DOOR: Tile = [23, 21]; // two wide, two tall
const HQ_ROOF = 16;

/** Light grey glass to a lit or a dark window. */
const GLASS: Record<string, [[number, number, number], [number, number, number]]> = {
  '217,221,230': [
    [255, 214, 112],
    [76, 88, 106],
  ],
  '209,214,225': [
    [244, 194, 92],
    [64, 74, 92],
  ],
  '222,226,233': [
    [255, 234, 164],
    [94, 106, 124],
  ],
};

export type Art = { sheet: HTMLImageElement; windows: HTMLCanvasElement };

/** The window tiles with lit and dark glass, side by side: brown frame lit, dark, then grey frame lit, dark. */
export function windowsOf(sheet: HTMLImageElement): HTMLCanvasElement {
  const canvas = document.createElement('canvas');
  canvas.width = 4 * T;
  canvas.height = T;
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  if (!ctx) return canvas;
  WINDOWS.forEach(([c, r], k) => {
    for (const lit of [0, 1]) ctx.drawImage(sheet, c * T, r * T, T, T, (2 * k + lit) * T, 0, T, T);
  });
  const img = ctx.getImageData(0, 0, 4 * T, T);
  for (let p = 0; p < img.data.length; p += 4) {
    const to = GLASS[`${img.data[p]},${img.data[p + 1]},${img.data[p + 2]}`];
    if (!to) continue;
    const [r, g, b] = to[Math.floor(p / 4 / T) % 2];
    [img.data[p], img.data[p + 1], img.data[p + 2]] = [r, g, b];
  }
  ctx.putImageData(img, 0, 0);
  return canvas;
}

const LIT = new Set<Activity>(['working', 'waking']);
/** Where window k sits on a building w wide, from its top-left. */
export const seatAt = (w: number, k: number): [number, number] => [1 + (k % (w - 2)), Math.floor(k / (w - 2))];

const noise = (x: number, y: number) => ((x * 73856093) ^ (y * 19349663)) >>> 0;

/** What the drawing steps share: the canvas, a tile put at a place (turned by quarters), and the lit and dark windows. */
type Pen = { ctx: CanvasRenderingContext2D; put: (t: Tile, x: number, y: number, turn?: number) => void; windows: HTMLCanvasElement };

/** Draws the whole city on a canvas at one pixel per sheet pixel. */
export function draw(ctx: CanvasRenderingContext2D, { sheet, windows }: Art, city: City) {
  ctx.imageSmoothingEnabled = false;
  const put = ([c, r]: Tile, x: number, y: number, turn = 0) => {
    if (!turn) return ctx.drawImage(sheet, c * T, r * T, T, T, x * T, y * T, T, T);
    ctx.save();
    ctx.translate((x + 0.5) * T, (y + 0.5) * T);
    ctx.rotate((turn * Math.PI) / 2);
    ctx.drawImage(sheet, c * T, r * T, T, T, -T / 2, -T / 2, T, T);
    ctx.restore();
  };
  const pen = { ctx, put, windows };
  drawStreets(pen, city);
  for (const p of city.parks) drawPark(pen, p);
  for (const lot of city.lots) {
    ctx.filter = lot.empty ? 'grayscale(1) brightness(0.92)' : 'none';
    if (lot.key === HQ) drawHq(pen, lot);
    else drawBuilding(pen, lot);
    ctx.filter = 'none';
  }
  // Street things last, over what they stand in front of: lamps along each block's sides, a tree on the curb in front of a building.
  for (const b of city.blocks)
    for (const x of [b.x, b.x + b.w - 1])
      for (const base of [b.y + 3, b.y + b.h - 2]) [16, 17, 18].forEach((r, k) => put([LAMP, r], x, base - 2 + k));
  for (const lot of city.lots) {
    const curbRow = lot.y + lot.h + 1;
    put(POTTED[lot.look % POTTED.length], lot.x + lot.w - 1, curbRow);
    if (lot.w >= 5 && lot.look % 3 === 0) put(BINS[lot.look % BINS.length], lot.x, curbRow);
  }
}

/** Asphalt, the blocks' pavement with its curbs, lane marks and crossings. */
function drawStreets({ ctx, put }: Pen, city: City) {
  // A curb along one side of a pavement tile: 0 bottom, 1 left, 2 top, 3 right.
  const curb = (x: number, y: number, side: number) => {
    const [px, py, pw, ph] = [
      [0, T - CURB_PX, T, CURB_PX],
      [0, 0, CURB_PX, T],
      [0, 0, T, CURB_PX],
      [T - CURB_PX, 0, CURB_PX, T],
    ][side];
    ctx.save();
    ctx.beginPath();
    ctx.rect(x * T + px, y * T + py, pw, ph);
    ctx.clip();
    put(CURB, x, y, side);
    ctx.restore();
  };
  for (let y = 0; y < city.h; y++) for (let x = 0; x < city.w; x++) put(ASPHALT, x, y);
  for (const b of city.blocks)
    for (let y = b.y; y < b.y + b.h; y++)
      for (let x = b.x; x < b.x + b.w; x++) {
        const n = noise(x, y);
        put(PAVEMENT[n % 3], x, y);
        if (y === b.y) curb(x, y, 2);
        if (y === b.y + b.h - 1) curb(x, y, 0);
        if (x === b.x) curb(x, y, 1);
        if (x === b.x + b.w - 1) curb(x, y, 3);
      }
  const avenueAt = (x: number) => city.avenues.some((a) => x >= a && x < a + ROAD);
  const roadAt = (y: number) => city.roads.some((r) => y >= r && y < r + ROAD);
  for (const r of city.roads)
    for (let x = 0; x < city.w; x++) {
      if (avenueAt(x)) continue;
      const crossing = avenueAt(x - 1) || avenueAt(x + 1);
      for (let k = 0; k < ROAD; k++) {
        if (crossing) put(ZEBRA_ACROSS_ROAD, x, r + k);
        else if (k === 1 && x % 2 === 0) put(LANE, x, r + k);
      }
    }
  for (const a of city.avenues)
    for (let y = 0; y < city.h; y++) {
      if (roadAt(y)) continue;
      const crossing = roadAt(y - 1) || roadAt(y + 1);
      for (let k = 0; k < ROAD; k++) {
        if (crossing) put(ZEBRA_ACROSS_AVENUE, a + k, y);
        else if (k === 1 && y % 2 === 0) put(LANE_UP, a + k, y);
      }
    }
}

/** Grass with trees and bushes, a bench, and a cart on the pavement in front. */
function drawPark({ put }: Pen, p: Area) {
  for (let y = p.y; y < p.y + p.h; y++) for (let x = p.x; x < p.x + p.w; x++) put(GRASS[noise(x, y) % 2], x, y);
  for (let x = p.x; x < p.x + p.w; x++) {
    const n = noise(x, p.y);
    if (n % 3 === 0) put(BUSHES[n % BUSHES.length], x, p.y + p.h - 1);
    if (x % 2 === 0 && p.h > 3) {
      const c = 31 + (n % 6);
      put([c, TREE_TOP], x, p.y + 1 + (n % 2));
      put([c, TREE_TOP + 1], x, p.y + 2 + (n % 2));
    }
  }
  if (p.w >= 4) put(BENCH, p.x + 1 + (noise(p.x, p.y) % (p.w - 2)), p.y + p.h - 2);
  if (p.w >= 3) [14, 15].forEach((r, k) => put([CARTS + (noise(p.x, p.y) % 3), r], p.x + 1, p.y + p.h + k));
}

/** A flat roof `rows` deep, a nine-slice from the corner set at column x0. */
function drawRoof({ put }: Pen, lot: Lot, x0: number, rows: number) {
  for (let j = 0; j < rows; j++)
    for (let i = 0; i < lot.w; i++) {
      const top = j === 0;
      const bottom = j === rows - 1;
      const left = i === 0;
      const right = i === lot.w - 1;
      const [c, r]: Tile =
        top && left
          ? [0, 0]
          : top && right
            ? [1, 0]
            : bottom && left
              ? [0, 1]
              : bottom && right
                ? [1, 1]
                : top
                  ? [2, 0]
                  : bottom
                    ? [2, 1]
                    : left
                      ? [3, 0]
                      : right
                        ? [3, 1]
                        : [4, 0];
      put([c + x0, r], lot.x + i, lot.y + j);
    }
}

/** A workspace: its roof, two floors of windows, a lit one per agent at work, and a door. */
function drawBuilding(pen: Pen, lot: Lot) {
  const { ctx, put, windows } = pen;
  const style = STYLES[lot.look % STYLES.length];
  const rows = lot.h - WALL;
  drawRoof(pen, lot, style.roof, rows);
  // Rooftop plant: a unit near one end, a second on a wide roof.
  put(ROOF_UNITS[lot.look % ROOF_UNITS.length], lot.x + 1 + (lot.look % 2) * (lot.w - 3), lot.y + rows - 2);
  if (lot.w >= 6) put(ROOF_UNITS[(lot.look >> 3) % ROOF_UNITS.length], lot.x + Math.floor(lot.w / 2), lot.y + rows - 1);
  WALL_ROWS.forEach((r, j) => {
    for (let i = 0; i < lot.w; i++) put([style.wall + (i === 0 ? 1 : i === lot.w - 1 ? 3 : 2), r], lot.x + i, lot.y + rows + j);
  });
  put([style.door[0], style.door[1] + (lot.look % 2)], lot.x + Math.floor(lot.w / 2), lot.y + rows + 2);
  lot.seats.slice(0, seatsOf(lot.w)).forEach((a, k) => {
    const [i, j] = seatAt(lot.w, k);
    const dark = lot.empty || !LIT.has(a) ? 1 : 0;
    ctx.drawImage(windows, (2 * style.window + dark) * T, 0, T, T, (lot.x + i) * T, (lot.y + rows + j) * T, T, T);
  });
}

/** The founder's glass HQ. */
function drawHq(pen: Pen, lot: Lot) {
  const { put } = pen;
  drawRoof(pen, lot, HQ_ROOF, 2);
  put(ROOF_UNITS[2], lot.x + 1, lot.y);
  GLASS_ROWS.forEach((r, j) => {
    for (let i = 0; i < lot.w; i++) put([i === 0 ? 13 : i === lot.w - 1 ? 15 : 14, r], lot.x + i, lot.y + 2 + j);
  });
  const d = lot.x + lot.w / 2 - 1;
  for (let k = 0; k < 4; k++) put([GLASS_DOOR[0] + (k % 2), GLASS_DOOR[1] + Math.floor(k / 2)], d + (k % 2), lot.y + 4 + Math.floor(k / 2));
}

// Cars seen from the side, 3 tiles by 2, facing left.
export const CARS: Tile[] = [
  [31, 16],
  [31, 20],
  [31, 24],
];
