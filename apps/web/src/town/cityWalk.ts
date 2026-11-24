// Where the founder walks in the city, in tiles: anywhere but into a building. A building's door is the middle of its front, on the
// pavement under its name; walking up into it, or clicking the building, goes in.
import type { City, Lot } from '#src/town/city.ts';
import type { Walk } from '#src/town/founder.tsx';
import { floorOf, type Spot } from '#src/town/walk.ts';

/** How tall the founder stands in the city, in tiles: a little under a storey. */
export const CITY_TALL = 1.4;
const DOOR_GAP = 1.3; // tiles below a building's front, past its name

export const doorOf = (lot: Lot): Spot => [lot.x + lot.w / 2, lot.y + lot.h + DOOR_GAP];

/** The city's walk, starting at the door of `from` (the HQ unless the founder just left a house). */
export function cityWalk(city: City, from: Lot | undefined): Walk {
  const solid = city.lots.map((l) => ({ x0: l.x, z0: l.y, x1: l.x + l.w, z1: l.y + l.h }));
  return {
    floor: floorOf(city.w, city.h, solid, { cell: 0.25, body: 0.3, top: 0 }),
    entry: from ? doorOf(from) : [city.w / 2, city.h / 2],
    speed: 4,
    stride: 0.4,
  };
}

/** The building the founder walks up into from `at`: the one whose front is just above, across its middle half. */
export function lotAbove(city: City, [x, y]: Spot): Lot | undefined {
  return city.lots.find((l) => Math.abs(x - (l.x + l.w / 2)) < l.w / 4 && y > l.y + l.h && y - (l.y + l.h) < 0.6);
}
