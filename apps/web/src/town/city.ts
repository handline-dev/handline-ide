// The city's layout, in tiles (street.ts builds it in metres). Streets cross in a grid: a strip of blocks with a road under it, an avenue
// between blocks. A workspace is a building facing the road, as wide as its team needs: a window per agent, lit while it works, the lead
// first. After the last building is the building site where a new team is added. What these leave of a block is a park.
import type { House } from '#src/town/model.ts';
import type { Activity } from '#src/layout/agentList.ts';

export const HQ = 'hq';
/** The building site's key among the buildings: no workspace has it. */
export const SITE = '+team';

// A strip from the top, in tiles: pavement behind the buildings, the buildings, pavement in front, the road.
const BACK = 1;
const BUILD = 6;
const FRONT = 2;
export const ROAD = 3;
const STRIP = BACK + BUILD + FRONT + ROAD;
const LOTS = 16; // a block's width for buildings
const SPAN = ROAD + 1 + LOTS + 1; // an avenue, then a block with pavement each side
const WALL = 3; // two floors of windows over the ground floor
const HQ_WIDTH = 6;
const SITE_WIDTH = 4;

/** A building as wide as its team: two floors of windows between its corner columns. */
export const widthFor = (people: number) => Math.min(7, Math.max(4, Math.ceil(people / 2) + 2));
export const seatsOf = (w: number) => 2 * (w - 2);

export type Lot = { key: string; x: number; y: number; w: number; h: number; seats: Activity[]; empty: boolean; look: number };
type Area = { x: number; y: number; w: number; h: number };
/** `site`: the building site, a lot with nothing built on it yet. */
export type City = { w: number; h: number; lots: Lot[]; site: Area; parks: Area[]; blocks: Area[]; roads: number[]; avenues: number[] };

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

/** The HQ, then every workspace, then the building site, in blocks: the site is the first free lot after the last building, in its block
 *  when that has room and in a block of its own when it is full. As many blocks to a strip as fill a pane of this shape (width over
 *  height) at the largest scale, and parks out to the pane's edges. */
export function cityOf(houses: House[], aspect: number): City {
  const items = [
    { key: HQ, w: HQ_WIDTH, seats: [] as Activity[], empty: false },
    ...houses.map((h) => {
      const seats = (h.lead ? [h.lead, ...h.people.filter((p) => p !== h.lead)] : h.people).map((p) => p.activity);
      return { key: h.key, w: widthFor(seats.length), seats, empty: h.working === 0 };
    }),
    { key: SITE, w: SITE_WIDTH, seats: [] as Activity[], empty: true },
  ];
  const groups = blocksOf(items.map((i) => i.w));
  let cols = 1;
  let best = 0;
  for (let c = 1; c <= groups.length; c++) {
    const [w, h] = sizeOf(c, Math.ceil(groups.length / c));
    const s = Math.min(aspect / w, 1 / h);
    if (s > best) [cols, best] = [c, s];
  }
  const [w, h] = sizeOf(cols, Math.ceil(groups.length / cols));

  const lots: Lot[] = [];
  let site: Area = { x: 0, y: 0, w: SITE_WIDTH, h: BUILD };
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
        if (it.key === SITE) site = { x, y: y0 + BACK, w: it.w, h: BUILD };
        else lots.push({ ...it, x, y: y0 + BACK + BUILD - height, h: height, look });
        x += it.w;
      }
      if (x0 + LOTS - x >= 2) parks.push({ x, y: y0 + BACK, w: x0 + LOTS - x, h: BUILD });
    }
  const roads = Array.from({ length: Math.ceil(h / STRIP) }, (_, s) => s * STRIP + BACK + BUILD + FRONT);
  const avenues = Array.from({ length: across + 1 }, (_, c) => c * SPAN);
  return { w, h, lots, site, parks, blocks, roads, avenues };
}
