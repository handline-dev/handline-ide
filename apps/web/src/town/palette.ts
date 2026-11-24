// The office's colours, the same in light and dark: warm walls and wood, white desks, teal sofas. A model's part takes its colour by the
// part's material name; parts not named here keep the colour the model came with.
import type { Color } from 'three';
import type { Kind, Thing } from '#src/town/room.ts';

type RGB = [number, number, number];

/** The names under the people: dark ink with a light halo, readable on the floor and the furniture. */
// eslint-disable-next-line no-restricted-syntax -- drawn art keeps its own colours in light and dark, not the theme's roles
const css = ([r, g, b]: RGB) => `rgb(${r} ${g} ${b})`;
export const NAME_INK = css([74, 64, 56]);
export const NAME_HALO = css([255, 250, 240]);

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
};
/** The cut along a wall's top: the top face of the walls takes it, so no thin box sits on the wall to show a sliver of its side. */
export const WALL_TOP: RGB = [96, 70, 58];
/** Surfaces drawn at their own colour whatever the sun does: walls and what is set in them. */
export const LIT_KINDS: Kind[] = ['wall', 'side', 'glass', 'frame', 'door'];

const PARTS: Record<string, Record<string, RGB>> = {
  desk: { wood: [236, 236, 232], metal: [196, 200, 208] },
  chairDesk: { metalMedium: [52, 56, 70], carpet: [62, 66, 84] },
  computerScreen: { metalDark: [44, 48, 62], metal: [150, 190, 235] },
  computerKeyboard: { metalDark: [60, 64, 78], metalMedium: [86, 90, 104] },
  computerMouse: { metalDark: [60, 64, 78] },
  pottedPlant: { wood: [240, 238, 232], woodDark: [110, 78, 56], plant: [88, 168, 72] },
  plantSmall2: { wood: [240, 238, 232], plant: [96, 176, 76] },
  bookcaseOpen: { wood: [196, 132, 80] },
  books: { carpetDarker: [70, 110, 190], carpetWhite: [240, 236, 224], plant: [230, 170, 60], metal: [200, 80, 80] },
  loungeSofa: { carpet: [112, 170, 160], wood: [120, 80, 50] },
  tableCoffee: { wood: [200, 142, 88] },
  rugRectangle: { carpet: [240, 228, 206], carpetDarker: [226, 210, 184] },
  kitchenCabinet: { wood: [236, 224, 206], woodDark: [196, 168, 132], metal: [150, 150, 160] },
};
const DARK_SCREEN: RGB = [58, 62, 78];
/** Parts that give off light: a screen while its agent works. */
const GLOWS = new Set(['computerScreen/metal']);

/** A model part's colour and whether it glows: a screen glows only while lit, and is dark otherwise. */
export function colourOf(t: Thing, material: string, own: Color): { colour: RGB; glow: boolean } {
  const glow = GLOWS.has(`${t.model}/${material}`) && (t.model !== 'computerScreen' || t.lit === true);
  if (t.model === 'computerScreen' && material === 'metal' && !t.lit) return { colour: DARK_SCREEN, glow: false };
  const hex = own.getHex(); // sRGB, as the palette is
  return { colour: PARTS[t.model]?.[material] ?? [(hex >> 16) & 255, (hex >> 8) & 255, hex & 255], glow };
}
