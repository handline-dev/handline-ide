// The town's colours, the same in light and dark: warm walls and floor boards, the colour of each kind of block. The furniture
// (furniture.ts) and the city (street.ts) give most of their blocks colours of their own.
import type { Kind } from '#src/town/room.ts';

type RGB = [number, number, number];

/** The names under the people: dark ink with a light halo, readable on the floor and the furniture. */
// eslint-disable-next-line no-restricted-syntax -- drawn art keeps its own colours in light and dark, not the theme's roles
const css = ([r, g, b]: RGB) => `rgb(${r} ${g} ${b})`;
export const NAME_INK = css([74, 64, 56]);
export const NAME_HALO = css([255, 250, 240]);
/** The legend's marks: a lit and a dark window, and a building nobody works in. */
export const SWATCH = { lit: css([255, 214, 112]), dark: css([76, 88, 106]), empty: css([168, 168, 168]) };
/** A building's name on its dark sign. */
export const SIGN_INK = css([244, 239, 226]);
/** What shows through an open door, a building's from the street or an office's: the dark beyond it. */
export const THROUGH = css([58, 50, 46]);

export const KIND_COLOUR: Record<Kind, RGB> = {
  floor: [226, 176, 124],
  slab: [120, 84, 62],
  wall: [236, 218, 196],
  side: [220, 198, 174],
  glass: [172, 214, 240],
  frame: [74, 80, 98],
  sill: [220, 196, 170],
  door: [176, 116, 70],
  baseboard: [200, 160, 120],
  paper: [248, 246, 238],
  road: [70, 72, 80],
  walk: [196, 200, 206],
  grass: [104, 168, 84],
  lane: [236, 236, 228],
  lit: [255, 214, 112],
  win: [76, 88, 106],
  trunk: [120, 84, 56],
  leaf: [84, 150, 70],
  thing: [200, 200, 200],
  rug: [240, 228, 206],
};
/** The cut along a wall's top: the top face of the walls takes it, so no thin box sits on the wall to show a sliver of its side. */
export const WALL_TOP: RGB = [96, 70, 58];
/** Surfaces drawn at their own colour whatever the sun does: walls and what is set in them. */
export const LIT_KINDS: Kind[] = ['wall', 'side', 'glass', 'frame', 'door', 'lit'];
