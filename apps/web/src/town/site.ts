// The building site where a new team is added, in metres, built from boxes as the rest of the city is (street.ts places it): a plot of
// bare earth fenced on three sides, open to the pavement, with a sign board on two posts at its front, a stack of bricks and a cone.
import { SITE } from '#src/town/city.ts';
import type { Put } from '#src/town/room.ts';

type RGB = [number, number, number];

const EARTH: RGB = [170, 140, 104];
const WOOD: RGB = [190, 152, 98];
const BOARD: RGB = [58, 62, 72];
const POST: RGB = [76, 82, 94];
const BRICK: RGB = [176, 86, 70];
const CONE: RGB = [236, 122, 48];

/** The site on the plot from x0 to x1 and from z0 back to its front at z = `front`, standing on a pavement `curb` high. The earth carries
 *  the building tag and the board the sign tag, so the site is found in the picture as a building is. */
export function buildSite(put: Put, [x0, x1]: [number, number], [z0, front]: [number, number], curb: number) {
  const part = `site:${x0}`;
  const mid = (x0 + x1) / 2;
  put([x0, curb, z0], [x1, curb + 0.02, front], 'grass', { colour: EARTH, tag: `house:${SITE}` });
  // The fence: a post every step along the back and down each side, two rails between them; nobody walks through it.
  const rail = (from: [number, number], to: [number, number]) => {
    put([from[0], curb, from[1]], [to[0], curb + 0.3, to[1]], 'side', { part, colour: WOOD, solid: true });
    for (const y of [0.34, 0.5]) put([from[0], curb + y, from[1]], [to[0], curb + y + 0.07, to[1]], 'side', { part, colour: WOOD });
  };
  rail([x0, z0], [x1, z0 + 0.05]);
  rail([x0, z0], [x0 + 0.05, front - 0.5]);
  rail([x1 - 0.05, z0], [x1, front - 0.5]);
  for (let x = x0; x <= x1 + 0.01; x += (x1 - x0) / 4)
    put([x - 0.04, curb, z0 - 0.02], [x + 0.04, curb + 0.66, z0 + 0.07], 'side', { part, colour: WOOD });
  for (const x of [x0, x1 - 0.05])
    for (const z of [z0 + (front - z0) / 2, front - 0.5])
      put([x - 0.02, curb, z - 0.04], [x + 0.07, curb + 0.66, z + 0.04], 'side', { part, colour: WOOD });
  // The board, as high over the pavement as a building's sign is over its door.
  for (const dx of [-0.8, 0.8])
    put([mid + dx - 0.04, curb, front - 0.1], [mid + dx + 0.04, 1.2, front - 0.02], 'side', { colour: POST, solid: true });
  put([mid - 0.95, 0.74, front - 0.02], [mid + 0.95, 1.12, front + 0.04], 'side', { colour: BOARD, tag: `sign:${SITE}` });
  // What is waiting to be built with.
  put([x0 + 0.35, curb, z0 + 0.5], [x0 + 1.05, curb + 0.3, z0 + 0.95], 'side', { colour: BRICK, pattern: 4, solid: true });
  put([x0 + 0.5, curb + 0.3, z0 + 0.55], [x0 + 0.95, curb + 0.5, z0 + 0.9], 'side', { colour: BRICK, pattern: 4 });
  put([x1 - 0.7, curb, front - 1.0], [x1 - 0.4, curb + 0.04, front - 0.7], 'side', { colour: CONE });
  put([x1 - 0.63, curb + 0.04, front - 0.93], [x1 - 0.47, curb + 0.4, front - 0.77], 'side', { colour: CONE, round: 'y' });
}
