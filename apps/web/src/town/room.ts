// A workspace's office as a 3D room, in metres (scene3d.ts draws it, people.ts puts the people in): a cut-open box with the back wall at
// z = 0, a desk per agent in rows of benches (the lead first), and a lounge of sofas right of them. An agent at work sits at its desk,
// its screen lit while it works or waits for the founder; an idle one sits in the lounge; one starting stands at the door; an asleep
// one's desk is empty. Furniture is Kenney's Furniture Kit (CC0), placed by the corner each model is built from.
import type { Activity } from '#src/layout/agentList.ts';
import type { House, Person } from '#src/town/model.ts';

/** A model from public/town/models at (x, z), turned `rot` radians about its origin; `tag` names it for the pixels that show it. */
export type Thing = { model: string; x: number; z: number; y?: number; rot?: number; scale?: number; tag?: string; lit?: boolean };
export type Kind = 'floor' | 'slab' | 'wall' | 'side' | 'glass' | 'frame' | 'sill' | 'door' | 'baseboard' | 'paper';
/** A plain box: its two corners and what it is; boxes of one `part` are one object, outlined only around the whole. */
export type Block = { from: [number, number, number]; to: [number, number, number]; kind: Kind; part?: string; tag?: string };
/** Where a person is: on a desk chair (its tag), on a sofa, or standing; (x, z) on the floor. */
export type Place = { person: Person; x: number; z: number; how: 'desk' | 'sofa' | 'door'; chair?: string };
export type Room = { w: number; d: number; wallH: number; people: Person[]; things: Thing[]; blocks: Block[]; places: Place[] };

export const WALL_H = 1.29;
export const WALL_T = 0.12;
export const SLAB = 0.22;
const PITCH = 0.734; // desks in a bench share an edge
const ROW = 1.75; // from one bench to the next, with room for the names between
const FIRST = 1.45; // the first bench's front edge
const SOFA = 0.98;
const SOFA_ROW = 1.6;
const DESK_TOP = 0.384;
const SHEET_T = 0.022; // one paper of a stack
const MOST_SHEETS = 14; // a stack stops growing here, however long the queue
/** The founder's desk's tag; it holds the founder's decisions. */
export const MY_DESK = 'desk:you';

const AT_DESK: Activity[] = ['working', 'you', 'held', 'error'];
const LIT: Activity[] = ['working', 'you'];

/** How many desks a bench may have: COLS[0] to COLS[1]. */
export const COLS = [2, 6] as const;

/** The room for a house: benches of `cols` desks (by default as many as keep the desks about square), the lounge as deep as its sitters
 *  need. */
export function roomOf(house: House, cols = Math.min(5, Math.max(COLS[0], Math.ceil(Math.sqrt(house.people.length))))): Room {
  const people = house.lead ? [house.lead, ...house.people.filter((p) => p !== house.lead)] : house.people;
  const rows = Math.max(1, Math.ceil(people.length / cols));
  const idle = people.filter((p) => p.activity === 'resting');
  const sofaRows = Math.max(1, Math.ceil(idle.length / 4));
  const x0 = 0.6;
  const lounge = x0 + cols * PITCH + 1.0;
  const w = lounge + 2 * SOFA + 1.1;
  // The founder's desk is a row of its own in front of the benches.
  const d = Math.max(FIRST + rows * ROW + 0.95, 1.75 + (sofaRows - 1) * SOFA_ROW + 1.55);
  const things: Thing[] = [];
  const places: Place[] = [];
  const papers: Block[] = [];
  const put = (model: string, x: number, z: number, more: Partial<Thing> = {}) => things.push({ model, x, z, ...more });
  /** A desk at (x, z) with a stack of `n` papers on its back right corner, each a little askew. */
  const desk = (x: number, z: number, tag: string, n: number) => {
    put('desk', x, z, { tag });
    for (let k = 0; k < Math.min(n, MOST_SHEETS); k++) {
      const [dx, dz] = [((k * 37) % 5) * 0.006, ((k * 23) % 3) * 0.006];
      const y = DESK_TOP + k * SHEET_T;
      papers.push({ from: [x + 0.47 + dx, y, z - 0.37 + dz], to: [x + 0.66 + dx, y + SHEET_T, z - 0.2 + dz], kind: 'paper', tag });
    }
  };

  // The benches: a desk, screen, keyboard and mouse per agent, its queue in papers, a plant on every other desk, the chair in front.
  people.forEach((p, i) => {
    const x = x0 + (i % cols) * PITCH;
    const z = FIRST + Math.floor(i / cols) * ROW;
    const chair = `chair:${p.key}`;
    desk(x, z, `desk:${p.key}`, p.queue);
    put('computerScreen', x + 0.17, z - 0.2, { y: 0.384, lit: LIT.includes(p.activity) });
    put('computerKeyboard', x + 0.22, z - 0.08, { y: 0.384 });
    put('computerMouse', x + 0.56, z - 0.06, { y: 0.384 });
    if (i % 2 === 0) put('plantSmall2', x + 0.07, z - 0.3, { y: 0.384, scale: 1.7 });
    put('chairDesk', x + 0.225, z + 0.3, { scale: 0.85, tag: chair });
    if (AT_DESK.includes(p.activity)) places.push({ person: p, x: x + 0.367, z: z + 0.2, how: 'desk', chair });
  });

  // The founder's desk, with a paper for every decision waiting on the founder and an empty chair.
  const mine = { x: x0 + ((cols - 1) * PITCH) / 2, z: FIRST + rows * ROW };
  desk(mine.x, mine.z, MY_DESK, house.decisions.length);
  put('computerScreen', mine.x + 0.17, mine.z - 0.2, { y: DESK_TOP });
  put('computerKeyboard', mine.x + 0.22, mine.z - 0.08, { y: DESK_TOP });
  put('chairDesk', mine.x + 0.225, mine.z + 0.3, { scale: 0.85, tag: MY_DESK });

  // The lounge: rows of two sofas, a rug and a coffee table in front of each; four sit on a row.
  for (let r = 0; r < sofaRows; r++) {
    const z = 1.75 + r * SOFA_ROW;
    put('rugRectangle', lounge - 0.2, z + 1.0);
    put('loungeSofa', lounge, z);
    put('loungeSofa', lounge + SOFA, z);
    put('tableCoffee', lounge + 0.71, z + 0.64);
  }
  idle.forEach((p, i) => {
    const r = Math.floor(i / 4);
    places.push({ person: p, x: lounge + 0.27 + (i % 4) * 0.48, z: 1.75 + r * SOFA_ROW - 0.13, how: 'sofa' });
  });
  put('pottedPlant', lounge - 0.25, 1.5, { scale: 1.1 });
  put('pottedPlant', lounge + 2 * SOFA + 0.2, 1.5, { scale: 1.2 });

  // Along the back wall: a bookcase, plants, a coffee corner, and the door at the right where whoever is starting walks in.
  put('bookcaseOpen', 0.05, 0.32);
  for (const y of [0.2, 0.42, 0.64]) {
    put('books', 0.1, 0.27, { y });
    put('books', 0.25, 0.27, { y, rot: 0.2 });
  }
  put('pottedPlant', 0.75, 0.25, { scale: 1.3 });
  const door = w - 1.15;
  put('kitchenCabinet', door - 0.95, 0.47);
  put('kitchenCoffeeMachine', door - 0.85, 0.32, { y: 0.45 });
  put('pottedPlant', door - 0.25, 0.25, { scale: 1.1 });
  people
    .filter((p) => p.activity === 'waking')
    .forEach((p, i) => places.push({ person: p, x: door + 0.35 - i * 0.4, z: 0.55, how: 'door' }));

  return { w, d, wallH: WALL_H, people, things, blocks: [...shell(w, d, door), ...papers], places };
}

/** The box itself: floor on a slab, three walls (palette.ts darkens their tops), a long window, a baseboard and the door. */
function shell(w: number, d: number, door: number): Block[] {
  const [t, h] = [WALL_T, WALL_H];
  const b: Block[] = [
    { from: [0, -0.01, 0], to: [w, 0, d], kind: 'floor' },
    { from: [-t, -SLAB, -t], to: [w + t, -0.01, d], kind: 'slab' },
    { from: [-t, 0, -t], to: [w + t, h, 0], kind: 'wall', part: 'back' },
    { from: [-t, 0, 0], to: [0, h, d], kind: 'side', part: 'left' },
    { from: [w, 0, 0], to: [w + t, h, d], kind: 'side', part: 'right' },
    { from: [0, 0, 0], to: [w, 0.06, 0.09], kind: 'baseboard' },
    { from: [door, 0, -0.02], to: [door + 0.7, 1.05, 0.015], kind: 'door' },
    { from: [door + 0.07, 0.62, -0.02], to: [door + 0.63, 0.98, 0.025], kind: 'glass' },
  ];
  // The window spans the wall from the bookcase to the coffee corner, in panes of about 0.9 m.
  const [wx0, wx1] = [1.15, door - 1.05];
  const panes = Math.max(1, Math.round((wx1 - wx0) / 0.89));
  const pane = (wx1 - wx0 - 0.05) / panes;
  b.push({ from: [wx0, 0.42, -0.02], to: [wx1, 1.18, 0.01], kind: 'frame' });
  for (let k = 0; k < panes; k++) {
    const gx = wx0 + 0.05 + k * pane;
    b.push({ from: [gx, 0.47, -0.019], to: [gx + pane - 0.05, 1.13, 0.02], kind: 'glass' });
  }
  b.push({ from: [wx0 - 0.05, 0.36, 0], to: [wx1 + 0.05, 0.42, 0.08], kind: 'sill' });
  return b;
}
