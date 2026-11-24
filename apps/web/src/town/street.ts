// The city in metres, built from boxes and cylinders as the office is (scene3d.ts draws it): city.ts's layout, a tile to U metres, with
// each workspace a building as wide as its team, a window per seat lit while that agent works, and the street things between them.
import { HQ, ROAD, type City, type Lot } from '#src/town/city.ts';
import type { Block, Kind, Room } from '#src/town/room.ts';

type RGB = [number, number, number];
type V3 = [number, number, number];

export const U = 0.75; // metres a tile
/** A person's height in the city's pixels: smaller than in an office, so more of the town is in view. */
export const CITY_PERSON_PX = 28;
const FH = 1.1; // a storey
const DEPTH = 2.4; // a building, front to back
const CURB = 0.1;

const STONE: RGB = [226, 218, 200];
const FRAME: RGB = [240, 238, 232];
const DARK: RGB = [58, 62, 72];
const METAL: RGB = [76, 82, 94];
/** Brick and roof colours by a building's look; even looks are brick, odd ones painted render. */
const LOOKS: { wall: RGB; roof: RGB; brick: boolean }[] = [
  { wall: [176, 86, 70], roof: [104, 64, 58], brick: true },
  { wall: [210, 200, 178], roof: [120, 116, 112], brick: false },
  { wall: [150, 96, 78], roof: [86, 74, 70], brick: true },
  { wall: [186, 204, 196], roof: [110, 120, 124], brick: false },
];

/** Blocks for one city, gathered by `put`; every builder below takes metres. */
function pen() {
  const blocks: Block[] = [];
  const put = (from: V3, to: V3, kind: Kind, more: Partial<Block> = {}) => blocks.push({ from, to, kind, ...more });
  return { blocks, put };
}
type Put = ReturnType<typeof pen>['put'];

/** A building on a lot, its front at z = `front`, named `name` on the sign over its door; `lit[k]` lights window k. */
function building(put: Put, lot: Lot, front: number) {
  const hq = lot.key === HQ;
  const look = LOOKS[lot.look % LOOKS.length];
  const [x0, x1] = [lot.x * U + 0.06, (lot.x + lot.w) * U - 0.06];
  const z0 = front - DEPTH;
  const floors = hq ? 4 : lot.h - 2;
  const top = floors * FH;
  const part = `b:${lot.key}`;
  const tag = `house:${lot.key}`;
  const wall = hq ? ([196, 220, 236] as RGB) : lot.empty ? ([168, 168, 168] as RGB) : look.wall;
  put([x0, 0, z0], [x1, top, front], 'wall', {
    solid: true,
    part,
    tag,
    colour: wall,
    top: look.roof,
    pattern: hq ? 7 : look.brick ? 4 : 0,
  });
  // The parapet round the roof, and a cornice over the front.
  const p = 0.12;
  put([x0, top, z0], [x1, top + 0.2, z0 + p], 'side', { part: `${part}:roof`, colour: STONE });
  put([x0, top, front - p], [x1, top + 0.2, front], 'side', { part: `${part}:roof`, colour: STONE });
  put([x0, top, z0], [x0 + p, top + 0.2, front], 'side', { part: `${part}:roof`, colour: STONE });
  put([x1 - p, top, z0], [x1, top + 0.2, front], 'side', { part: `${part}:roof`, colour: STONE });
  put([x0 - 0.04, top - 0.12, front], [x1 + 0.04, top, front + 0.08], 'side', { colour: STONE });
  // On the roof: an air unit with its fan, and a vent.
  const ax = x0 + 0.35 + (lot.look % 3) * 0.3;
  put([ax, top, z0 + 0.5], [ax + 0.6, top + 0.42, z0 + 1.1], 'side', { colour: [196, 200, 206] });
  put([ax + 0.12, top + 0.42, z0 + 0.62], [ax + 0.48, top + 0.46, z0 + 0.98], 'side', { colour: DARK, round: 'y' });
  put([x1 - 0.6, top, z0 + 0.6], [x1 - 0.45, top + 0.5, z0 + 0.75], 'side', { colour: METAL, round: 'y' });

  const mid = (x0 + x1) / 2;
  if (hq) {
    // Glass doors, and the name over them.
    put([mid - 0.6, 0, front], [mid + 0.6, 0.95, front + 0.04], 'frame', { colour: METAL });
    put([mid - 0.55, 0, front + 0.01], [mid - 0.02, 0.9, front + 0.05], 'glass', { colour: [150, 190, 220], pattern: 0 });
    put([mid + 0.02, 0, front + 0.01], [mid + 0.55, 0.9, front + 0.05], 'glass', { colour: [150, 190, 220], pattern: 0 });
    put([mid - 0.7, 1.12, front], [mid + 0.7, 1.36, front + 0.06], 'side', { colour: DARK, tag: `sign:${lot.key}` });
    return;
  }
  // Floor bands between the storeys.
  for (let f = 1; f < floors; f++) put([x0, f * FH - 0.04, front], [x1, f * FH + 0.04, front + 0.05], 'side', { colour: STONE });
  // A window per seat over the ground floor, the top storey first: frame, glass (lit while its agent works) and a sill.
  const across = lot.w - 2;
  for (let r = 0; r < floors - 1; r++)
    for (let c = 0; c < across; c++) {
      const k = r * across + c;
      const cx = (lot.x + 1.5 + c) * U;
      const y = (floors - 1 - r) * FH + 0.28;
      const lit = lot.seats[k] === 'working' || lot.seats[k] === 'waking';
      // The glass stands out from the wall inside a frame of four bars standing farther out, so no two faces meet.
      const part = `win:${lot.key}:${k}`;
      put([cx - 0.17, y + 0.05, front], [cx + 0.17, y + 0.57, front + 0.025], lit ? 'lit' : 'win');
      put([cx - 0.22, y, front], [cx - 0.17, y + 0.62, front + 0.06], 'frame', { part, colour: FRAME });
      put([cx + 0.17, y, front], [cx + 0.22, y + 0.62, front + 0.06], 'frame', { part, colour: FRAME });
      put([cx - 0.17, y + 0.57, front], [cx + 0.17, y + 0.62, front + 0.06], 'frame', { part, colour: FRAME });
      put([cx - 0.17, y, front], [cx + 0.17, y + 0.05, front + 0.06], 'frame', { part, colour: FRAME });
      put([cx - 0.012, y + 0.05, front], [cx + 0.012, y + 0.57, front + 0.04], 'frame', { colour: FRAME });
      put([cx - 0.26, y - 0.06, front], [cx + 0.26, y, front + 0.1], 'side', { colour: STONE });
    }
  // The ground floor: a shop window each side of a glass door, the sign on the wall over them. Nothing stands out from the front, so
  // seen from high nothing hides the door or the sign.
  put([x0 + 0.15, 0.08, front], [x1 - 0.15, 0.76, front + 0.03], 'frame', { colour: DARK });
  put([x0 + 0.2, 0.13, front + 0.01], [mid - 0.3, 0.71, front + 0.035], 'win', { colour: [120, 150, 176] });
  put([mid + 0.3, 0.13, front + 0.01], [x1 - 0.2, 0.71, front + 0.035], 'win', { colour: [120, 150, 176] });
  put([mid - 0.25, 0, front + 0.01], [mid + 0.25, 0.76, front + 0.04], 'door', { colour: [150, 100, 64] });
  put([mid - 0.18, 0.36, front + 0.04], [mid + 0.18, 0.68, front + 0.05], 'win', { colour: [120, 150, 176] });
  put([mid - 0.7, 0.8, front], [mid + 0.7, 1.02, front + 0.06], 'side', { colour: DARK, tag: `sign:${lot.key}` });
}

/** A street lamp at (x, z), its arm reaching toward +z. */
function lamp(put: Put, x: number, z: number) {
  const part = `lamp:${x},${z}`;
  put([x - 0.09, 0, z - 0.09], [x + 0.09, 0.14, z + 0.09], 'side', { part, colour: DARK, round: 'y', solid: true });
  put([x - 0.04, 0, z - 0.04], [x + 0.04, 2.2, z + 0.04], 'side', { part, colour: DARK, round: 'y' });
  put([x - 0.03, 2.14, z - 0.03], [x + 0.03, 2.2, z + 0.4], 'side', { part, colour: DARK });
  put([x - 0.15, 2.04, z + 0.22], [x + 0.15, 2.18, z + 0.52], 'side', { part, colour: DARK });
  put([x - 0.12, 1.96, z + 0.25], [x + 0.12, 2.04, z + 0.49], 'lit', { colour: [255, 232, 160] });
}

/** A tree in a square of soil at (x, z): a trunk under three tiers of leaves. */
function tree(put: Put, x: number, z: number) {
  put([x - 0.2, CURB, z - 0.2], [x + 0.2, CURB + 0.01, z + 0.2], 'side', { colour: [96, 74, 56] });
  put([x - 0.06, CURB, z - 0.06], [x + 0.06, 1.0, z + 0.06], 'trunk', { round: 'y', solid: true });
  const part = `tree:${x},${z}`;
  put([x - 0.46, 0.85, z - 0.46], [x + 0.46, 1.3, z + 0.46], 'leaf', { part, colour: [70, 132, 64], round: 'y' });
  put([x - 0.38, 1.3, z - 0.38], [x + 0.38, 1.7, z + 0.38], 'leaf', { part, colour: [84, 150, 70], round: 'y' });
  put([x - 0.22, 1.7, z - 0.22], [x + 0.22, 1.95, z + 0.22], 'leaf', { part, colour: [104, 168, 82], round: 'y' });
}

/** A bench along x at (x, z), facing +z: legs, three seat slats, a back of two slats and arms. */
function bench(put: Put, x: number, z: number) {
  const part = `bench:${x},${z}`;
  const wood: RGB = [176, 120, 74];
  for (const dx of [-0.55, 0.49]) {
    put([x + dx, CURB, z - 0.18], [x + dx + 0.06, CURB + 0.26, z + 0.18], 'side', { part, colour: DARK });
    put([x + dx, CURB + 0.26, z - 0.2], [x + dx + 0.06, CURB + 0.6, z - 0.14], 'side', { part, colour: DARK });
    put([x + dx, CURB + 0.38, z - 0.16], [x + dx + 0.06, CURB + 0.42, z + 0.16], 'side', { part, colour: DARK });
  }
  for (let i = 0; i < 3; i++)
    put([x - 0.6, CURB + 0.26, z - 0.15 + i * 0.11], [x + 0.6, CURB + 0.3, z - 0.07 + i * 0.11], 'side', {
      part,
      colour: wood,
      solid: true,
    });
  for (const y of [0.38, 0.5]) put([x - 0.6, CURB + y, z - 0.2], [x + 0.6, CURB + y + 0.07, z - 0.16], 'side', { part, colour: wood });
}

/** A litter bin at (x, z): a green drum with a lid and a band. */
function bin(put: Put, x: number, z: number) {
  const part = `bin:${x},${z}`;
  put([x - 0.14, CURB, z - 0.14], [x + 0.14, CURB + 0.46, z + 0.14], 'side', { part, colour: [52, 104, 80], round: 'y', solid: true });
  put([x - 0.16, CURB + 0.46, z - 0.16], [x + 0.16, CURB + 0.5, z + 0.16], 'side', { part, colour: [40, 80, 62], round: 'y' });
  put([x - 0.15, CURB + 0.3, z - 0.15], [x + 0.15, CURB + 0.33, z + 0.15], 'side', { part, colour: [40, 80, 62], round: 'y' });
}

/** Traffic lights at (x, z) facing +z: a pole, a dark head, red lit and the others dim. */
function lights(put: Put, x: number, z: number) {
  const part = `lights:${x},${z}`;
  put([x - 0.045, 0, z - 0.045], [x + 0.045, 2.0, z + 0.045], 'side', { part, colour: DARK, round: 'y', solid: true });
  put([x - 0.1, 1.45, z - 0.04], [x + 0.1, 2.0, z + 0.1], 'side', { part, colour: [40, 42, 48] });
  [
    ['lit', [240, 72, 60]],
    ['side', [110, 96, 50]],
    ['side', [50, 100, 64]],
  ].forEach(([kind, colour], i) =>
    put([x - 0.055, 1.86 - i * 0.16, z + 0.1], [x + 0.055, 1.97 - i * 0.16, z + 0.13], kind as Kind, { colour: colour as RGB, round: 'z' }),
  );
}

/** A small car along x at (x, z): body, cabin with glass all round, four wheels, head and tail lights. */
function car(put: Put, x: number, z: number, colour: RGB, facing: 1 | -1) {
  const part = `car:${x},${z}`;
  const [l, w] = [1.5, 0.74];
  put([x - l / 2, 0.14, z - w / 2], [x + l / 2, 0.46, z + w / 2], 'side', { part, colour, solid: true });
  const cab = x - facing * 0.12;
  put([cab - 0.42, 0.46, z - w / 2 + 0.05], [cab + 0.42, 0.74, z + w / 2 - 0.05], 'side', { part, colour });
  put([cab - 0.36, 0.5, z + w / 2 - 0.05], [cab - 0.02, 0.7, z + w / 2 - 0.03], 'win', { colour: [92, 120, 150] });
  put([cab + 0.02, 0.5, z + w / 2 - 0.05], [cab + 0.36, 0.7, z + w / 2 - 0.03], 'win', { colour: [92, 120, 150] });
  put([cab - 0.36, 0.5, z - w / 2 + 0.03], [cab + 0.36, 0.7, z - w / 2 + 0.05], 'win', { colour: [92, 120, 150] });
  // Bumpers front and back.
  put([x - l / 2 - 0.03, 0.14, z - w / 2 + 0.04], [x + l / 2 + 0.03, 0.22, z + w / 2 - 0.04], 'side', { colour: [60, 62, 70] });
  put([cab + facing * 0.42 - 0.02, 0.5, z - w / 2 + 0.08], [cab + facing * 0.42 + 0.02, 0.7, z + w / 2 - 0.08], 'win', {
    colour: [92, 120, 150],
  });
  for (const dx of [-0.48, 0.48])
    for (const dz of [-w / 2, w / 2 - 0.1])
      put([x + dx - 0.15, 0, z + dz], [x + dx + 0.15, 0.3, z + dz + 0.1], 'side', { colour: [30, 30, 34], round: 'z' });
  const head = x + (facing * l) / 2;
  put([head - 0.03, 0.3, z - w / 2 + 0.06], [head + 0.03, 0.38, z - w / 2 + 0.18], 'lit', { colour: [255, 246, 210] });
  put([head - 0.03, 0.3, z + w / 2 - 0.18], [head + 0.03, 0.38, z + w / 2 - 0.06], 'lit', { colour: [255, 246, 210] });
  const tail = x - (facing * l) / 2;
  put([tail - 0.03, 0.3, z - w / 2 + 0.06], [tail + 0.03, 0.38, z - w / 2 + 0.16], 'side', { colour: [200, 60, 56] });
  put([tail - 0.03, 0.3, z + w / 2 - 0.16], [tail + 0.03, 0.38, z + w / 2 - 0.06], 'side', { colour: [200, 60, 56] });
}

/** Zebra stripes across a road from (x0, z) to (x1, z), each running along z for `long`. */
function zebra(put: Put, x0: number, x1: number, z: number, long: number) {
  for (let x = x0; x + 0.25 <= x1; x += 0.45) put([x, 0, z], [x + 0.25, 0.004, z + long], 'lane');
}

/** Where a building is entered: the spot on the pavement before its door, and its front's span. */
export type Door = { key: string; at: [number, number]; x0: number; x1: number; front: number };
export type CityRoom = Room & { doors: Door[] };

const CAR_COLOURS: RGB[] = [
  [70, 120, 200],
  [220, 96, 72],
  [240, 196, 80],
  [96, 170, 120],
  [230, 230, 226],
];

/** The whole city: asphalt under everything, each block's pavement, buildings, parks and street things, lane marks and a crossing at every
 *  corner, lights where streets meet, and `cars` cars on the roads. */
export function cityRoom(city: City, cars: number): CityRoom {
  const { blocks, put } = pen();
  const [W, D] = [city.w * U, city.h * U];
  const road = ROAD * U;
  put([0, -0.02, 0], [W, 0, D], 'road', { pattern: 6 });
  const onRoad = (z: number) => city.roads.some((r) => z > r * U - 0.1 && z < (r + ROAD) * U + 0.1);
  const onAvenue = (x: number) => city.avenues.some((a) => x > a * U - 0.1 && x < (a + ROAD) * U + 0.1);
  for (const r of city.roads) {
    const z = (r + ROAD / 2) * U;
    for (let x = 0.3; x < W - 0.6; x += 1.2)
      if (!onAvenue(x) && !onAvenue(x + 0.6)) put([x, 0, z - 0.04], [x + 0.6, 0.004, z + 0.04], 'lane');
    // A crossing just past each avenue, where people step off the corner.
    for (const a of city.avenues) zebra(put, (a + ROAD) * U + 0.2, (a + ROAD) * U + 2.2, r * U + 0.2, road - 0.4);
  }
  for (const a of city.avenues) {
    const x = (a + ROAD / 2) * U;
    for (let z = 0.3; z < D - 0.6; z += 1.2) if (!onRoad(z) && !onRoad(z + 0.6)) put([x - 0.04, 0, z], [x + 0.04, 0.004, z + 0.6], 'lane');
  }
  const doors: Door[] = [];
  for (const b of city.blocks) {
    const [bx0, bz0, bx1, bz1] = [b.x * U, b.y * U, (b.x + b.w) * U, (b.y + b.h) * U];
    put([bx0, 0, bz0], [bx1, CURB, bz1], 'walk', { pattern: 5 });
    put([bx0 - 0.08, 0, bz1 - 0.08], [bx1 + 0.08, CURB, bz1 + 0.04], 'walk', { colour: [176, 178, 184] });
    const inside = (x: number, y: number) => x >= b.x && x < b.x + b.w && y >= b.y && y < b.y + b.h;
    const lots = city.lots.filter((l) => inside(l.x, l.y));
    const front = (b.y + b.h - 2) * U;
    for (const l of lots) {
      building(put, l, front);
      const mid = (l.x + l.w / 2) * U;
      doors.push({ key: l.key, at: [mid, front + 0.9], x0: mid - 0.4, x1: mid + 0.4, front });
    }
    // A park where the buildings leave room: grass, rows of trees, a bench facing the street.
    for (const p of city.parks.filter((p) => inside(p.x, p.y))) {
      const [px0, px1] = [p.x * U, (p.x + p.w) * U];
      put([px0, CURB, front - DEPTH - 0.6], [px1, CURB + 0.02, front], 'grass');
      for (let x = px0 + 0.7; x < px1 - 0.5; x += 1.4) {
        tree(put, x, front - 2.5);
        tree(put, x + 0.7, front - 1.0);
      }
      if (px1 - px0 > 2) bench(put, (px0 + px1) / 2, front + 0.5);
    }
    // Along the curb: lamps at the ends, bins between the buildings, a bench and a bin by the HQ, lights at the corner; no tree, which
    // seen from high would hide a front.
    const curbZ = bz1 - 0.35;
    lamp(put, bx0 + 0.9, curbZ);
    lamp(put, bx1 - 0.35, curbZ);
    lights(put, bx0 + 0.25, bz1 - 0.2);
    lots.forEach((l, i) => {
      const mid = (l.x + l.w / 2) * U;
      if (l.key === HQ) {
        bench(put, mid + 1.4, front + 0.55);
        bin(put, mid + 2.35, front + 0.45);
      } else if (i % 2 === 0) bin(put, (l.x + l.w) * U - 0.5, curbZ);
    });
  }
  // Cars, a road each in turn, spread along it, the near lane driving right and the far one left.
  for (let i = 0; i < cars; i++) {
    const r = city.roads[i % city.roads.length];
    const near = i % 2 === 0;
    const x = ((i * 7.3 + 3) % (W - 4)) + 2;
    // Clear of the avenues and the crossing just past each.
    if (!onAvenue(x - 0.9) && !onAvenue(x + 0.9) && !onAvenue(x - 3.2))
      car(put, x, (r + (near ? 0.72 : 0.3) * ROAD) * U, CAR_COLOURS[i % CAR_COLOURS.length], near ? 1 : -1);
  }
  const tallest = Math.max(1, ...city.lots.map((l) => (l.key === HQ ? 4 : l.h - 2) * FH + 0.2));
  const hq = doors.find((d) => d.key === HQ);
  return {
    distance: 200,
    personPx: CITY_PERSON_PX,
    pitch: (68 * Math.PI) / 180,
    w: W,
    d: D,
    wallH: tallest,
    people: [],
    things: [],
    blocks,
    places: [],
    entry: hq?.at ?? [W / 2, D / 2],
    doors,
  };
}
