// Where each office sprite sits in public/town/office.png: [x, y, width, height]. A pose is the first figure's; figure i is
// FIGURE * i rows below it. Generated from the sprite parts; regenerate, do not edit.
export const SPRITES = {
  desk_on: [124, 0, 44, 34],
  desk_off: [169, 0, 44, 34],
  chair_back: [214, 0, 22, 30],
  chair_front: [237, 0, 24, 30],
  chair_side: [262, 0, 20, 30],
  sofa: [0, 40, 53, 30],
  table: [71, 40, 36, 23],
  mailbox: [104, 0, 19, 35],
  door: [22, 0, 26, 38],
  plant: [0, 0, 21, 39],
  plant_s: [54, 40, 16, 25],
  window: [49, 0, 54, 37],
  bubble: [108, 40, 19, 19],
  down: [0, 71, 26, 41],
  up: [27, 71, 26, 40],
  left: [54, 71, 25, 39],
  right: [80, 71, 26, 39],
  walk_down: [107, 71, 33, 39],
  walk_right: [141, 71, 33, 39],
  sit_back: [175, 71, 26, 36],
  sit_front: [202, 71, 25, 37],
  sit_side: [228, 71, 29, 36],
  sit_sofa: [258, 71, 29, 36],
} as const satisfies Record<string, readonly [number, number, number, number]>;

export const FIGURES = 6;
export const FIGURE = 42;
export const [SHEET_W, SHEET_H] = [287, 322];
