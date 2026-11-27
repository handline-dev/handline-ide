// A workspace's office as a 3D room, in metres (scene3d.ts draws it, people.ts puts the people in): a cut-open box with the back wall at
// z = 0, rows of benches (the lead first) in the work room on the left, and on the right two rooms, each parted from it by a low wall with
// a doorway in the middle: the rest room of sofas at the back, and at the front the way in, the founder's desk ahead of the door. An agent
// at work sits at its desk, its screen lit while it works or waits for the founder; an idle one stays at its desk while its prompt cache lives
// and then sits in the rest room; one starting stands by the founder's desk. One more desk, empty, is where the founder hires. Furniture is in furniture.ts.
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
/** A place's name, different once the desk or sofa stands elsewhere. */
export const seatName = (p: Place) => `${p.how}:${p.x.toFixed(2)},${p.z.toFixed(2)}`;
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
  /** The HQ's spot where the next agent hired would stand (fit.ts). */
  join?: [number, number];
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
/** From a desk's left edge to the middle of its chair, where its sitter is. */
export const CHAIR_MID = 0.367;
const ROW = 1.75; // from one bench to the next, with room for the names between
const FIRST = 1.45; // the first bench's front edge
const SOFA = 0.98;
const SOFA_ROW = 1.6;
const FRONT_H = 1.0; // the front wall, lower than the others: it is seen face on, from the side the founder comes in by
const DOOR_W = 0.55;
const DOOR_H = 0.95;
const FRONT = 0.8; // the floor the front wall covers seen from above, added to the room's depth so nobody sits under it
const INNER_H = 0.5; // the walls inside the office: low, so what is behind one and the doorways in it stay in sight
const DOORWAY = 0.9; // the opening in the middle of each right room's wall to the work room
const REST_TO_DESK = 0.35; // from the rest room's wall to the back of the founder's desk
const DESK_D = 0.39; // a desk, front to back (furniture.ts)
const LOUNGE = 1.4; // the first row of sofas, clear of the coffee corner
const STAND_ROW = 0.9; // from one row of standing people to the next
const SHEET_T = 0.022; // one paper of a stack
const MOST_SHEETS = 14; // a stack stops growing here, however long the queue
/** The founder's desk's tag; it holds the founder's decisions. */
export const MY_DESK = 'desk:you';
/** The founder's chair's tag. */
export const MY_CHAIR = 'chair:you';
/** The tag of the empty desk a team's office keeps for the next agent, and of its chair: a click there hires one. */
export const HIRE_DESK = 'desk:hire';
/** The tag of the stack of papers on a desk (by the desk's tag): its queued tasks, or the founder's decisions. */
export const papersOf = (desk: string) => `papers:${desk.slice('desk:'.length)}`;
/** The tag of a working agent's screen, whose picture moves (monitors.tsx). */
export const screenOf = (key: string) => `screen:${key}`;
/** The tag of the door the founder comes in and goes out by; a building's in the city is `door:<its key>`. */
export const DOOR = 'door';
const FRONT_PART = 'wall:front';
/** Whether a block is the front wall or its door, which an office draws over the people (Inside.tsx). */
export const inFront = (b: Block) => b.part === FRONT_PART || b.tag === DOOR;

const AT_DESK: Activity[] = ['working', 'you', 'held', 'error'];
/** Whose screens are lit: working, or waiting for the founder. */
export const LIT: Activity[] = ['working', 'you'];

/** The room for a house: benches of `cols` desks on the left; right of them the rest room at the back and the way in at the front, the
 *  founder's desk ahead of the door, each half the depth unless the sofas need more; `fit` widens the work room by `w` metres and makes
 *  the room at least `d` deep (fit.ts shapes it to the picture). The HQ has no benches: its people (the pinned agents) stand together,
 *  `cols` to a row, with room for one more. The screens of `dark` stay dark: they are still walking to their desks. */
export function roomOf(
  house: House,
  cols = Math.min(5, Math.max(2, Math.ceil(Math.sqrt(house.people.length)))),
  fit = { w: 0, d: 0 },
  dark: ReadonlySet<string> = new Set(),
): Room {
  const people = house.lead ? [house.lead, ...house.people.filter((p) => p !== house.lead)] : house.people;
  const hq = house.hq === true;
  // A team's benches hold one desk more than it has agents, and the HQ one spot more: the empty one, where the founder hires.
  const rows = Math.max(1, Math.ceil((people.length + 1) / cols));
  // Resting while its prompt cache lives, an agent stays at its desk; after, it goes to a sofa.
  const seated = (p: Person) => AT_DESK.includes(p.activity) || (p.activity === 'resting' && p.cached);
  const idle = hq ? [] : people.filter((p) => p.activity === 'resting' && !p.cached);
  const sofaRows = Math.max(1, Math.ceil(idle.length / 4));
  const x0 = 0.6;
  const benches = x0 + cols * PITCH;
  // Right of the benches a column of two rooms: the rest room at the back, and the way in under it with the door in the middle.
  const col = benches + 0.9 + fit.w;
  const lounge = col + 0.45;
  const w = lounge + 2 * SOFA + 0.45;
  const door = (col + w - DOOR_W) / 2;
  const last = hq ? FIRST + (rows - 1) * STAND_ROW : FIRST + (rows - 1) * ROW;
  const mineD = 0.95; // from the founder's desk's front edge to the front wall's floor
  const entry = WALL_T + REST_TO_DESK + DESK_D + mineD; // the way in, from the rest room's wall to the front wall's floor
  const sofas = LOUNGE + (sofaRows - 1) * SOFA_ROW + 1.3; // the rest room's least depth
  const d = Math.max(fit.d, FRONT + Math.max(last + 1.35, sofas + entry));
  // The two rooms share the column's depth half and half, the rest room more only when its sofas need it.
  const rest = Math.max(sofas, (d - FRONT - WALL_T) / 2);
  const places: Place[] = [];
  const { blocks, put } = pen();
  /** A desk at (x, z) with a stack of `n` papers on its back right corner, each a little askew. */
  const desk = (x: number, z: number, tag: string, n: number) => {
    F.desk(put, x, z, tag);
    for (let k = 0; k < Math.min(n, MOST_SHEETS); k++) {
      const [dx, dz] = [((k * 37) % 5) * 0.006, ((k * 23) % 3) * 0.006];
      const y = F.DESK_TOP + k * SHEET_T;
      put([x + 0.57 + dx, y, z - 0.37 + dz], [x + 0.7 + dx, y + SHEET_T, z - 0.2 + dz], 'paper', { tag: papersOf(tag) });
    }
  };

  // The benches: a desk, screen, keyboard and mouse per agent, its queue in papers, a plant on every other desk, the chair in front.
  if (hq)
    people.forEach((p, i) => {
      places.push({ person: p, x: x0 + 0.37 + (i % cols) * PITCH, z: FIRST + Math.floor(i / cols) * STAND_ROW, how: 'door' });
    });
  else {
    people.forEach((p, i) => {
      const x = x0 + (i % cols) * PITCH;
      const z = FIRST + Math.floor(i / cols) * ROW;
      const chair = `chair:${p.key}`;
      desk(x, z, `desk:${p.key}`, p.queue);
      const lit = LIT.includes(p.activity) && !dark.has(p.key);
      F.screen(put, x + 0.17, z - 0.2, F.DESK_TOP, lit, lit && p.activity === 'working' ? screenOf(p.key) : undefined);
      F.keyboard(put, x + 0.22, z - 0.08, F.DESK_TOP, 0.34);
      if (i % 2 === 0) F.plant(put, x + 0.07, z - 0.3, F.DESK_TOP, 0.24);
      F.chair(put, x + 0.225, z + 0.3, chair);
      if (seated(p)) places.push({ person: p, x: x + CHAIR_MID, z: z + 0.2, how: 'desk', chair });
    });
    // The desk after the last agent's, where the next agent's would stand: an empty chair at a dark screen. The agents' desks are laid
    // out as if it were not there, so nobody's moves for it; only the room grows a row when the last bench is full.
    const [hx, hz] = [x0 + (people.length % cols) * PITCH, FIRST + Math.floor(people.length / cols) * ROW];
    desk(hx, hz, HIRE_DESK, 0);
    F.screen(put, hx + 0.17, hz - 0.2, F.DESK_TOP, false);
    F.keyboard(put, hx + 0.22, hz - 0.08, F.DESK_TOP, 0.34);
    F.chair(put, hx + 0.225, hz + 0.3, HIRE_DESK);
  }

  // The founder's desk, with a paper for every decision waiting on the founder and an empty chair: straight ahead of the door
  // in the middle of its room's depth, but never nearer the door than mineD.
  const mine = { x: door + DOOR_W / 2 - CHAIR_MID, z: Math.min(d - FRONT - mineD, (rest + WALL_T + d - FRONT) / 2 + 0.2) };
  desk(mine.x, mine.z, MY_DESK, house.decisions.length);
  F.screen(put, mine.x + 0.17, mine.z - 0.2, F.DESK_TOP, false);
  F.keyboard(put, mine.x + 0.22, mine.z - 0.08, F.DESK_TOP);
  F.chair(put, mine.x + 0.225, mine.z + 0.3, MY_CHAIR);

  // The rest room: rows of two sofas, a rug and a coffee table in front of each; four sit on a row. A coffee corner on its back wall.
  for (let r = 0; r < sofaRows; r++) {
    const z = LOUNGE + r * SOFA_ROW;
    F.rug(put, lounge + 0.2, z + 1.0);
    F.sofa(put, lounge, z);
    F.sofa(put, lounge + SOFA, z);
    F.coffeeTable(put, lounge + 0.65, z + 0.74);
  }
  idle.forEach((p, i) => {
    const r = Math.floor(i / 4);
    places.push({ person: p, x: lounge + 0.27 + (i % 4) * 0.48, z: LOUNGE + r * SOFA_ROW - 0.13, how: 'sofa' });
  });
  F.plant(put, col + 0.25, 0.25, 0, 0.72);
  F.cabinet(put, lounge + 0.55, 0.47);
  F.coffeeMachine(put, lounge + 0.65, 0.32, 0.45);
  F.cabinet(put, lounge + 0.98, 0.47);
  F.plant(put, w - 0.25, 0.25, 0, 0.78);

  // Along the work room's back wall: a bookcase and a plant; whoever is starting stands beside the founder's desk, two on each side,
  // clear of the door, a row further back for every four more.
  F.bookcase(put, 0.05, 0.32);
  F.plant(put, 0.75, 0.25, 0, 0.85);
  const stand = [mine.x - 0.35, mine.x - 0.75, mine.x + 1.08, mine.x + 1.48];
  (hq ? [] : people)
    .filter((p) => p.activity === 'waking')
    .forEach((p, i) => places.push({ person: p, x: stand[i % 4], z: mine.z + 0.25 - Math.floor(i / 4) * 0.45, how: 'door' }));

  const all = [...shell(w, d, door, col), ...rightRooms(col, w, d, rest), ...blocks];
  return { w, d, wallH: WALL_H, people, blocks: all, places, entry: [door + DOOR_W / 2, d - 0.45] };
}

/** The low walls of the right rooms: the rest room's to the way in, and the one between both rooms and the work room, from the back wall
 *  to the front wall, open in the middle of each room's length (the front wall's cover is not the room's). */
function rightRooms(col: number, w: number, d: number, rest: number): Block[] {
  const side = (z0: number, z1: number): Block => ({
    from: [col, 0, z0],
    to: [col + WALL_T, INNER_H, z1],
    kind: 'side',
    part: 'wall:right-rooms',
    solid: true,
  });
  /** The wall along [z0, z1] with the doorway in the middle of [z0, mid1]. */
  const opened = (z0: number, z1: number, mid1: number) => {
    const mid = (z0 + mid1) / 2;
    return [side(z0, mid - DOORWAY / 2), side(mid + DOORWAY / 2, z1)];
  };
  return [
    { from: [col, 0, rest], to: [w, INNER_H, rest + WALL_T], kind: 'wall', part: 'wall:rest', solid: true },
    ...opened(0, rest, rest),
    ...opened(rest, d - WALL_T, d - FRONT),
  ];
}

/** The box itself: floor on a slab, four walls (palette.ts darkens their tops), a long window over the work room, a baseboard, and the
 *  front wall with the door the founder comes up through. The front wall's blocks are `inFront`: an office draws them as a layer of their
 *  own over the people. */
function shell(w: number, d: number, door: number, col: number): Block[] {
  const [t, h] = [WALL_T, WALL_H];
  const b: Block[] = [
    { from: [0, -0.01, 0], to: [w, 0, d], kind: 'floor' },
    { from: [-t, -SLAB, -t], to: [w + t, -0.01, d], kind: 'slab' },
    { from: [-t, 0, -t], to: [w + t, h, 0], kind: 'wall', part: 'back' },
    { from: [-t, 0, 0], to: [0, h, d], kind: 'side', part: 'left' },
    { from: [w, 0, 0], to: [w + t, h, d], kind: 'side', part: 'right' },
    { from: [0, 0, 0], to: [w, 0.06, 0.09], kind: 'baseboard' },
    { from: [-t, 0, d - t], to: [door, FRONT_H, d], kind: 'wall', part: FRONT_PART, solid: true },
    { from: [door + DOOR_W, 0, d - t], to: [w + t, FRONT_H, d], kind: 'wall', part: FRONT_PART, solid: true },
    { from: [door, DOOR_H, d - t], to: [door + DOOR_W, FRONT_H, d], kind: 'wall', part: FRONT_PART },
    { from: [door, 0, d - 0.05], to: [door + DOOR_W, DOOR_H, d - 0.01], kind: 'door', tag: DOOR },
    { from: [door + 0.06, 0.55, d - 0.055], to: [door + DOOR_W - 0.06, 0.85, d - 0.005], kind: 'glass', tag: DOOR },
  ];
  // The window spans the work room's back wall from the bookcase to near the rest room, in panes of about 0.9 m.
  const [wx0, wx1] = [1.15, col - 0.3];
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
