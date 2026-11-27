// The people in the 3D room, as the image model drew them: public/town/people.png holds a row per person and a column per pose, each
// figure bottom-centred in its cell at half the generated size, with only the background taken out. They are drawn over the room's
// pixels at the screen's own resolution, never snapped to the room's grid. One at its desk sits with its back to us; an idle one sits on
// a sofa with a mug; one starting walks in at the door on a small shadow. Whatever of the room stands nearer than a person (its chair,
// a sofa's arm) is laid back over it. Each person is one of the sheet's people, teammates different ones while there are enough.
import type { Person } from '#src/town/model.ts';
import { HIRE_DESK, MY_DESK, papersOf, type Room } from '#src/town/room.ts';
import type { Frame } from '#src/town/scene3d.ts';
import type { Picture } from '#src/town/shade.ts';
import { Vector3 } from 'three';
import { PERSON_PX } from '#src/town/view.ts';

export const PEOPLE_SHEET = `${import.meta.env.BASE_URL}town/people.png`;
export const BUBBLE = `${import.meta.env.BASE_URL}town/bubble.png`;
export const BUBBLE_PX = 19;
/** The sheet's cells, and each person's figure height per pose in sheet pixels (from the script that cut the sheet). */
export const SHEET = { cell: [140, 232], size: [420, 1392], walk: 220 } as const;
/** The walking sheet, public/town/people-walk.png: a row per person, in the order of the sitting sheet, and nine columns: down, up and
 *  left, each standing, left foot, right foot (right is left mirrored). Every cell keeps one empty row under the feet. */
export const WALK_SHEET = `${import.meta.env.BASE_URL}town/people-walk.png`;
export const WALK = { cell: [156, 234], size: [1404, 1404] } as const;
export type Facing = 'down' | 'up' | 'left' | 'right';
const WALK_COL: Record<Facing, number> = { down: 0, up: 3, left: 6, right: 6 };
const WALK_STEPS = [0, 1, 0, 2] as const;
/** The column of the walking sheet for a facing and how far the person has walked, in strides: standing, left, standing, right. */
export const walkColumn = (facing: Facing, strides: number): number => WALK_COL[facing] + WALK_STEPS[((Math.floor(strides) % 4) + 4) % 4];
const POSES = ['sit_back', 'sit_mug', 'walk_down'] as const;
const TALL: Record<Pose, number[]> = {
  sit_back: [195, 226, 194, 227, 195, 201],
  sit_mug: [208, 226, 206, 222, 208, 202],
  walk_down: [220, 226, 220, 230, 220, 211],
};
const PEOPLE = TALL.walk_down.length;
export const SCALE = PERSON_PX / SHEET.walk; // a walking figure of median height stands PERSON_PX tall in the room
type Pose = (typeof POSES)[number];
/** How high a turned sitter's figure stands its bottom: the seat's height, less what the knees hang below it. */
const SEAT = 0.12;

/** A box in picture pixels. */
export type Box = { x: number; y: number; w: number; h: number };
/** What a click opens an agent's chat from, in picture pixels: its figure, or its desk with its chair; `at` is the floor spot (metres)
 *  the founder walks to first. */
export type Spot = Box & { person: Person; at: [number, number] };
/** A person's name, centred on x with its top at y; `away` under the empty desk of one who is not in the room. */
export type Name = { person: Person; x: number; y: number; away?: boolean };
/** A person drawn: its cell of the sheet (`col`, `row`) in `box`, the room's nearer pixels to lay over it as an image in `front`, and how
 *  far into the room it is (`z`), so the founder walks in front of or behind it. */
export type Figure = { person: Person; col: number; row: number; box: Box; z: number; front?: { url: string } & Box };
/** Where a mark over a person goes: the middle of its head's top, or of its chair's when nobody sits at its desk. */
export type Head = { key: string; x: number; y: number };
/** `mine`: the founder's desk, with where its name goes. */
export type Cast = {
  figures: Figure[];
  heads: Head[];
  bubbles: (Box & { key: string })[];
  spots: Spot[];
  desks: Spot[];
  /** The stacks of papers on agents' desks: their queued tasks. */
  papers: Spot[];
  names: Name[];
  mine?: Box & { name: [number, number]; at: [number, number] };
  /** The empty desk where the founder hires the next agent, with where its label goes. */
  hire?: Box & { name: [number, number]; at: [number, number] };
};

/** What lies flat on the floor, under a standing person's feet. */
export const FLAT = new Set(['floor', 'baseboard', 'rug', 'road', 'walk', 'grass', 'lane']);

const hashOf = (key: string) => [...key].reduce((n, ch) => (n * 31 + ch.charCodeAt(0)) >>> 0, 7);

/** A sheet row per person from its key, so it keeps it: the first from there on that no teammate has yet. */
export function figuresOf(people: Person[]): Map<string, number> {
  const taken = new Set<number>();
  return new Map(
    people.map((p) => {
      const start = hashOf(p.key) % PEOPLE;
      const figure = Array.from({ length: PEOPLE }, (_, i) => (start + i) % PEOPLE).find((f) => !taken.has(f)) ?? start;
      taken.add(figure);
      return [p.key, figure];
    }),
  );
}

/** The pixels a room object covers, by its tag, as a box. */
/** Every tagged object's box, in one pass over the picture. */
export function boxesOf(pic: Picture, frame: Frame): Map<string, Box> {
  const ends = new Map<string, [number, number, number, number]>();
  for (let i = 0; i < pic.id.length; i++) {
    const tag = frame.meta[pic.id[i]]?.tag;
    if (!tag) continue;
    const [x, y] = [i % pic.w, Math.floor(i / pic.w)];
    const e = ends.get(tag);
    if (e) [e[0], e[1], e[2], e[3]] = [Math.min(e[0], x), Math.min(e[1], y), Math.max(e[2], x), Math.max(e[3], y)];
    else ends.set(tag, [x, y, x, y]);
  }
  return new Map([...ends].map(([tag, [x0, y0, x1, y1]]) => [tag, { x: x0, y: y0, w: x1 - x0 + 1, h: y1 - y0 + 1 }]));
}

export function boxOf(pic: Picture, frame: Frame, tag: string): Box | null {
  let [x0, y0, x1, y1] = [Infinity, Infinity, -1, -1];
  for (let i = 0; i < pic.id.length; i++) {
    if (frame.meta[pic.id[i]]?.tag !== tag) continue;
    const [x, y] = [i % pic.w, Math.floor(i / pic.w)];
    [x0, y0, x1, y1] = [Math.min(x0, x), Math.min(y0, y), Math.max(x1, x), Math.max(y1, y)];
  }
  return x1 < 0 ? null : { x: x0, y: y0, w: x1 - x0 + 1, h: y1 - y0 + 1 };
}

/** How far from the camera one standing upright at (x, z) is at each picture row: a row higher up is a point higher on its body, which
 *  the camera looking down sees nearer. A chair's back just behind someone is nearer than their waist but farther than their head, and
 *  covers neither. */
export function depthByRow(
  toPixel: (x: number, y: number, z: number) => [number, number],
  depthOf: (x: number, y: number, z: number) => number,
  x: number,
  z: number,
): (row: number) => number {
  const [[, feet], [, metre]] = [toPixel(x, 0, z), toPixel(x, 1, z)];
  const [low, high] = [depthOf(x, 0, z), depthOf(x, 1, z)];
  return (row) => low + ((high - low) * (feet - row)) / (feet - metre);
}

/** The room's pixels inside `box` that `covers` picks, with where they start; none when nothing is picked. */
export function frontPixels(pic: Picture, box: Box, covers: (i: number) => boolean): { img: ImageData; x: number; y: number } | undefined {
  const [x0, y0] = [Math.max(0, Math.floor(box.x)), Math.max(0, Math.floor(box.y))];
  const [x1, y1] = [Math.min(pic.w, Math.ceil(box.x + box.w)), Math.min(pic.h, Math.ceil(box.y + box.h))];
  if (x1 <= x0 || y1 <= y0) return undefined;
  const img = new ImageData(x1 - x0, y1 - y0);
  let any = false;
  for (let y = y0; y < y1; y++) {
    for (let x = x0; x < x1; x++) {
      const i = y * pic.w + x;
      if (!pic.id[i] || !covers(i)) continue;
      img.data.set(pic.rgba.subarray(i * 4, i * 4 + 4), ((y - y0) * img.width + x - x0) * 4);
      any = true;
    }
  }
  return any ? { img, x: x0, y: y0 } : undefined;
}

/** The same pixels as a small image, for a person who sits still. */
function frontOf(pic: Picture, box: Box, covers: (i: number) => boolean): Figure['front'] {
  const px = frontPixels(pic, box, covers);
  if (!px) return undefined;
  const canvas = document.createElement('canvas');
  [canvas.width, canvas.height] = [px.img.width, px.img.height];
  canvas.getContext('2d')?.putImageData(px.img, 0, 0);
  return { url: canvas.toDataURL(), x: px.x, y: px.y, w: px.img.width, h: px.img.height };
}

/** Where every placed person is drawn, back to front, and where each is for clicks and names; door shadows go into the picture. */
export function castOf(pic: Picture, frame: Frame, room: Room): Cast {
  const cast: Cast = { figures: [], heads: [], bubbles: [], spots: [], desks: [], papers: [], names: [] };
  const rows = figuresOf(room.people);
  const depthAt = (x: number, z: number) => {
    const [px, py] = frame.toPixel(x, 0.5, z).map(Math.round);
    return pic.depth[py * pic.w + px] ?? 1e9;
  };
  /** The cell box for a pose with its figure's feet (or seat) centred at (cx, bottom). */
  const boxFor = (pose: Pose, cx: number, bottom: number): Box => {
    const [w, h] = [SHEET.cell[0] * SCALE, SHEET.cell[1] * SCALE];
    return { x: cx - w / 2, y: bottom + SCALE - h, w, h }; // a cell keeps one empty row under its figure
  };

  /** Right beside a desk's chair, at its left where the desks of a bench part, level with whoever sits in it: where the founder stands
   *  to talk. */
  const beside = (tag: string): [number, number] => {
    const top = room.blocks.find((b) => b.tag === tag && b.part?.startsWith('desk:'));
    return top ? [top.from[0], top.to[2] + 0.2] : room.entry;
  };
  const depthOf = (x: number, y: number, z: number) => -new Vector3(x, y, z).applyMatrix4(frame.camera.matrixWorldInverse).z;

  for (const place of [...room.places].sort((a, b) => a.z - b.z)) {
    const p = place.person;
    const row = rows.get(p.key) ?? 0;
    const draw = (pose: Pose, cx: number, bottom: number, covers: (i: number) => boolean) => {
      const box = boxFor(pose, cx, bottom);
      const tall = TALL[pose][row] * SCALE;
      const figure: Figure = { person: p, col: POSES.indexOf(pose), row, box, z: place.z, front: frontOf(pic, box, covers) };
      cast.figures.push(figure);
      cast.heads.push({ key: p.key, x: cx, y: bottom - tall });
      return { x: cx - box.w / 2, y: bottom - tall, w: box.w, h: tall };
    };
    if (place.how === 'desk' && place.chair) {
      const chair = boxOf(pic, frame, place.chair);
      if (!chair) continue;
      // Head and shoulders show over the chair's back, the head's top 17 px above the chair's top whatever the figure's height: low
      // enough to leave the screen in view.
      const tall = TALL.sit_back[row] * SCALE;
      // Only the sitter's own chair covers it, and anything nearer than that chair: the desk and screen behind it never do.
      const tag = place.chair;
      let near = Infinity;
      for (let i = 0; i < pic.id.length; i++) if (frame.meta[pic.id[i]]?.tag === tag) near = Math.min(near, pic.depth[i]);
      const seen = draw(
        'sit_back',
        chair.x + chair.w / 2,
        chair.y - 17 + tall,
        (i) => frame.meta[pic.id[i]]?.tag === tag || pic.depth[i] < near - 0.05,
      );
      if (p.activity === 'you') cast.bubbles.push({ key: p.key, x: seen.x + seen.w - 8, y: seen.y - 6, w: BUBBLE_PX, h: BUBBLE_PX });
      cast.names.push({ person: p, x: chair.x + chair.w / 2, y: chair.y + chair.h + 1 });
      continue;
    }
    if (place.how === 'turned' && place.chair) {
      // Turned round in its chair, the sitter faces the room from the seat's front edge; only what stands nearer than that covers it.
      const [fx, fy] = frame.toPixel(place.x, SEAT, place.z);
      const depth = depthByRow(frame.toPixel, depthOf, place.x, place.z);
      const seen = draw('sit_mug', fx, fy, (i) => pic.depth[i] < depth(Math.floor(i / pic.w)) - 0.05);
      cast.spots.push({ person: p, ...seen, at: beside(`desk:${p.key}`) });
      cast.names.push({ person: p, x: seen.x + seen.w / 2, y: seen.y + seen.h + 1 });
      continue;
    }
    const sofa = place.how === 'sofa';
    const [fx, fy] = frame.toPixel(place.x, sofa ? 0.1 : 0, place.z);
    if (!sofa) shadowAt(pic, frame, place.x, place.z);
    // A sitter sinks into the sofa: only what stands well in front of it hides it. One standing is hidden by what stands nearer than its
    // body, never by the floor.
    const depth = depthAt(place.x, place.z);
    const body = -new Vector3(place.x, 0.45, place.z).applyMatrix4(frame.camera.matrixWorldInverse).z;
    const seen = sofa
      ? draw('sit_mug', fx, fy, (i) => pic.depth[i] < depth - 0.6)
      : draw('walk_down', fx, fy, (i) => !FLAT.has(frame.meta[pic.id[i]]?.kind ?? '') && pic.depth[i] < body);
    cast.spots.push({ person: p, ...seen, at: sofa ? [place.x, place.z + 0.55] : [place.x - 0.367, place.z] });
    cast.names.push({ person: p, x: seen.x + seen.w / 2, y: seen.y + seen.h + 1 });
  }
  for (const person of room.people) {
    const desk = boxOf(pic, frame, `desk:${person.key}`);
    if (desk) cast.desks.push({ ...desk, person, at: beside(`desk:${person.key}`) });
    const papers = boxOf(pic, frame, papersOf(`desk:${person.key}`));
    if (papers) cast.papers.push({ ...papers, person, at: beside(`desk:${person.key}`) });
    // Nobody at this desk: a mark for its agent goes over the empty chair.
    const chair = cast.heads.some((h) => h.key === person.key) ? null : boxOf(pic, frame, `chair:${person.key}`);
    if (chair) cast.heads.push({ key: person.key, x: chair.x + chair.w / 2, y: chair.y });
    // Its name still stands under the chair, so one can tell whose empty desk it is.
    if (chair) cast.names.push({ person, x: chair.x + chair.w / 2, y: chair.y + chair.h + 1, away: true });
  }
  const hire = boxOf(pic, frame, HIRE_DESK);
  if (hire) cast.hire = { ...hire, name: [hire.x + hire.w / 2, hire.y + hire.h + 1], at: beside(HIRE_DESK) };
  // The founder's desk opens their decisions whether its papers or the desk itself is clicked.
  const mine = union(boxOf(pic, frame, MY_DESK), boxOf(pic, frame, papersOf(MY_DESK)));
  if (mine) cast.mine = { ...mine, name: [mine.x + mine.w / 2, mine.y + mine.h + 1], at: beside(MY_DESK) };
  return cast;
}

/** The box round two boxes, either of them missing. */
function union(a: Box | null, b: Box | null): Box | null {
  if (!a || !b) return a ?? b;
  const [x, y] = [Math.min(a.x, b.x), Math.min(a.y, b.y)];
  return { x, y, w: Math.max(a.x + a.w, b.x + b.w) - x, h: Math.max(a.y + a.h, b.y + b.h) - y };
}

/** A soft dark oval on the floor under a standing person. */
function shadowAt(pic: Picture, frame: Frame, x: number, z: number) {
  const [cx, cy] = frame.toPixel(x, 0, z);
  const [ex] = frame.toPixel(x + 0.13, 0, z);
  const [, ey] = frame.toPixel(x, 0, z + 0.07);
  const [rx, ry] = [Math.max(1, ex - cx), Math.max(1, ey - cy)];
  for (let y = Math.floor(cy - ry); y <= cy + ry; y++) {
    for (let px = Math.floor(cx - rx); px <= cx + rx; px++) {
      if (((px - cx) / rx) ** 2 + ((y - cy) / ry) ** 2 > 1 || px < 0 || y < 0 || px >= pic.w || y >= pic.h) continue;
      const i = (y * pic.w + px) * 4;
      pic.rgba.set([pic.rgba[i] * 0.82, pic.rgba[i + 1] * 0.8, pic.rgba[i + 2] * 0.84, pic.rgba[i + 3]], i);
    }
  }
}
