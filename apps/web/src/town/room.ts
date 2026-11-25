// A workspace's office as a 3D room, in metres (scene3d.ts draws it, people.ts puts the people in): a cut-open box with the back wall at
// z = 0, a desk per agent in rows of benches (the lead first), and a lounge of sofas right of them. An agent at work sits at its desk,
// its screen lit while it works or waits for the founder; an idle one sits in the lounge; one starting stands at the door; an asleep
// one's desk is empty. The furniture is built in code (furniture.ts).
import type { Activity } from '#src/layout/agentList.ts';
import * as F from '#src/town/furniture.ts';
import type { House, Person } from '#src/town/model.ts';

export type Kind =
  | 'floor'
  | 'slab'
  | 'wall'
  | 'side'
  | 'glass'
  | 'frame'
  | 'sill'
  | 'door'
  | 'baseboard'
  | 'paper'
  | 'road'
  | 'walk'
  | 'grass'
  | 'lane'
  | 'lit'
  | 'win'
  | 'trunk'
  | 'leaf'
  | 'thing'
  | 'rug';
/** A plain box: its two corners and what it is; boxes of one `part` are one object, outlined only around the whole. */
/** `colour` and `top` (its upward faces) set a block's colours over its kind's, `pattern` its surface (scene3d.ts); `round` makes it a
 *  cylinder filling the box along that axis. */
export type Block = {
  from: [number, number, number];
  to: [number, number, number];
  kind: Kind;
  part?: string;
  tag?: string;
  colour?: [number, number, number];
  top?: [number, number, number];
  pattern?: number;
  round?: 'x' | 'y' | 'z';
  /** The founder walks round it. */
  solid?: boolean;
};
/** Where a person is: on a desk chair (its tag), on a sofa, or standing (`door`, as one starting does inside the door, and the HQ's
 *  people do); (x, z) on the floor. */
export type Place = { person: Person; x: number; z: number; how: 'desk' | 'sofa' | 'door'; chair?: string };
/** `entry`: where the founder comes in, inside the door. */
/** `distance`, `pitch` and `personPx`: how far the camera stands, in metres, how steeply it looks down, in radians, and how many
 *  pixels tall a person is drawn (view.ts). */
export type Room = {
  distance?: number;
  pitch?: number;
  personPx?: number;
  w: number;
  d: number;
  wallH: number;
  people: Person[];
  blocks: Block[];
  places: Place[];
  entry: [number, number];
};

/** Blocks gathered by `put`, in metres. */
export function pen() {
  const blocks: Block[] = [];
  const put = (from: [number, number, number], to: [number, number, number], kind: Kind, more: Partial<Block> = {}) =>
    blocks.push({ from, to, kind, ...more });
  return { blocks, put };
}
export type Put = ReturnType<typeof pen>['put'];

export const WALL_H = 1.29;
export const WALL_T = 0.12;
export const SLAB = 0.22;
const PITCH = 0.734; // desks in a bench share an edge
const ROW = 1.75; // from one bench to the next, with room for the names between
const FIRST = 1.45; // the first bench's front edge
const SOFA = 0.98;
const SOFA_ROW = 1.6;
const STAND_ROW = 0.9; // from one row of standing people to the next
const SHEET_T = 0.022; // one paper of a stack
const MOST_SHEETS = 14; // a stack stops growing here, however long the queue
/** The founder's desk's tag; it holds the founder's decisions. */
export const MY_DESK = 'desk:you';
/** The tag of the door the founder comes in and goes out by; a building's in the city is `door:<its key>`. */
export const DOOR = 'door';

const AT_DESK: Activity[] = ['working', 'you', 'held', 'error'];
const LIT: Activity[] = ['working', 'you'];

/** How many desks a bench may have: COLS[0] to COLS[1]. */
export const COLS = [2, 6] as const;

/** The room for a house: benches of `cols` desks (by default as many as keep the desks about square), the lounge as deep as its sitters
 *  need. The HQ has no benches: its people (the pinned agents) stand together, `cols` to a row, wherever their desks are. */
export function roomOf(house: House, cols = Math.min(5, Math.max(COLS[0], Math.ceil(Math.sqrt(house.people.length))))): Room {
  const people = house.lead ? [house.lead, ...house.people.filter((p) => p !== house.lead)] : house.people;
  const rows = Math.max(1, Math.ceil(people.length / cols));
  const hq = house.hq === true;
  const idle = hq ? [] : people.filter((p) => p.activity === 'resting');
  const sofaRows = Math.max(1, Math.ceil(idle.length / 4));
  const x0 = 0.6;
  const lounge = x0 + cols * PITCH + 1.0;
  const w = lounge + 2 * SOFA + 1.1;
  // The founder's desk is a row of its own in front of the benches, or of those standing in the HQ.
  const front = hq ? FIRST + rows * STAND_ROW + 0.4 : FIRST + rows * ROW;
  const d = Math.max(front + 0.95, 1.75 + (sofaRows - 1) * SOFA_ROW + 1.55);
  const places: Place[] = [];
  const { blocks, put } = pen();
  /** A desk at (x, z) with a stack of `n` papers on its back right corner, each a little askew. */
  const desk = (x: number, z: number, tag: string, n: number) => {
    F.desk(put, x, z, tag);
    for (let k = 0; k < Math.min(n, MOST_SHEETS); k++) {
      const [dx, dz] = [((k * 37) % 5) * 0.006, ((k * 23) % 3) * 0.006];
      const y = F.DESK_TOP + k * SHEET_T;
      put([x + 0.57 + dx, y, z - 0.37 + dz], [x + 0.7 + dx, y + SHEET_T, z - 0.2 + dz], 'paper', { tag });
    }
  };

  // The benches: a desk, screen, keyboard and mouse per agent, its queue in papers, a plant on every other desk, the chair in front.
  if (hq)
    people.forEach((p, i) => {
      places.push({ person: p, x: x0 + 0.37 + (i % cols) * PITCH, z: FIRST + Math.floor(i / cols) * STAND_ROW, how: 'door' });
    });
  else
    people.forEach((p, i) => {
      const x = x0 + (i % cols) * PITCH;
      const z = FIRST + Math.floor(i / cols) * ROW;
      const chair = `chair:${p.key}`;
      desk(x, z, `desk:${p.key}`, p.queue);
      F.screen(put, x + 0.17, z - 0.2, F.DESK_TOP, LIT.includes(p.activity));
      F.keyboard(put, x + 0.22, z - 0.08, F.DESK_TOP, 0.34);
      if (i % 2 === 0) F.plant(put, x + 0.07, z - 0.3, F.DESK_TOP, 0.24);
      F.chair(put, x + 0.225, z + 0.3, chair);
      if (AT_DESK.includes(p.activity)) places.push({ person: p, x: x + 0.367, z: z + 0.2, how: 'desk', chair });
    });

  // The founder's desk, with a paper for every decision waiting on the founder and an empty chair.
  const mine = { x: x0 + ((cols - 1) * PITCH) / 2, z: front };
  desk(mine.x, mine.z, MY_DESK, house.decisions.length);
  F.screen(put, mine.x + 0.17, mine.z - 0.2, F.DESK_TOP, false);
  F.keyboard(put, mine.x + 0.22, mine.z - 0.08, F.DESK_TOP);
  F.chair(put, mine.x + 0.225, mine.z + 0.3, MY_DESK);

  // The lounge: rows of two sofas, a rug and a coffee table in front of each; four sit on a row.
  for (let r = 0; r < sofaRows; r++) {
    const z = 1.75 + r * SOFA_ROW;
    F.rug(put, lounge - 0.2, z + 1.0);
    F.sofa(put, lounge, z);
    F.sofa(put, lounge + SOFA, z);
    F.coffeeTable(put, lounge + 0.25, z + 0.74);
  }
  idle.forEach((p, i) => {
    const r = Math.floor(i / 4);
    places.push({ person: p, x: lounge + 0.27 + (i % 4) * 0.48, z: 1.75 + r * SOFA_ROW - 0.13, how: 'sofa' });
  });
  F.plant(put, lounge - 0.25, 1.5, 0, 0.72);
  F.plant(put, lounge + 2 * SOFA + 0.2, 1.5, 0, 0.78);

  // Along the back wall: a bookcase, plants, a coffee corner, and the door at the right where whoever is starting walks in.
  F.bookcase(put, 0.05, 0.32);
  F.plant(put, 0.75, 0.25, 0, 0.85);
  const door = w - 1.15;
  F.cabinet(put, door - 0.95, 0.47);
  F.coffeeMachine(put, door - 0.85, 0.32, 0.45);
  F.plant(put, door - 0.25, 0.25, 0, 0.72);
  (hq ? [] : people)
    .filter((p) => p.activity === 'waking')
    .forEach((p, i) => places.push({ person: p, x: door + 0.35 - i * 0.4, z: 0.55, how: 'door' }));

  return { w, d, wallH: WALL_H, people, blocks: [...shell(w, d, door), ...blocks], places, entry: [door + 0.35, 0.45] };
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
    { from: [door, 0, -0.02], to: [door + 0.7, 1.05, 0.015], kind: 'door', tag: DOOR },
    { from: [door + 0.07, 0.62, -0.02], to: [door + 0.63, 0.98, 0.025], kind: 'glass', tag: DOOR },
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
