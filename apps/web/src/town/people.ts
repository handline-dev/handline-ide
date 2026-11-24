// The people in the 3D room, as the image model drew them: public/town/people.png holds a row per person and a column per pose, each
// figure bottom-centred in its cell at half the generated size, with only the background taken out. They are drawn over the room's
// pixels at the screen's own resolution, never snapped to the room's grid. One at its desk sits with its back to us; an idle one sits on
// a sofa with a mug; one starting walks in at the door on a small shadow. Whatever of the room stands nearer than a person (its chair,
// a sofa's arm) is laid back over it. Each person is one of the sheet's people, teammates different ones while there are enough.
import type { Person } from '#src/town/model.ts';
import { MY_DESK, type Room } from '#src/town/room.ts';
import type { Frame } from '#src/town/scene3d.ts';
import type { Picture } from '#src/town/shade.ts';
import { PERSON_PX } from '#src/town/view.ts';

export const PEOPLE_SHEET = `${import.meta.env.BASE_URL}town/people.png`;
export const BUBBLE = `${import.meta.env.BASE_URL}town/bubble.png`;
export const BUBBLE_PX = 19;
/** The sheet's cells, and each person's figure height per pose in sheet pixels (from the script that cut the sheet). */
export const SHEET = { cell: [140, 232], size: [420, 1392], walk: 220 } as const;
const POSES = ['sit_back', 'sit_mug', 'walk_down'] as const;
const TALL: Record<Pose, number[]> = {
  sit_back: [195, 226, 194, 227, 195, 201],
  sit_mug: [208, 226, 206, 222, 208, 202],
  walk_down: [220, 226, 220, 230, 220, 211],
};
const PEOPLE = TALL.walk_down.length;
const SCALE = PERSON_PX / SHEET.walk; // a walking figure of median height stands PERSON_PX tall in the room
type Pose = (typeof POSES)[number];

/** A box in picture pixels. */
type Box = { x: number; y: number; w: number; h: number };
/** What a click opens an agent's chat from, in picture pixels: its figure, or its desk with its chair. */
export type Spot = Box & { person: Person };
/** A person's name, centred on x with its top at y. */
export type Name = { person: Person; x: number; y: number };
/** A person drawn: its cell of the sheet (`col`, `row`) in `box`, and the room's nearer pixels to lay over it, as an image in `front`. */
export type Figure = { person: Person; col: number; row: number; box: Box; front?: { url: string } & Box };
/** `mine`: the founder's desk, with where its name goes. */
export type Cast = {
  figures: Figure[];
  bubbles: Box[];
  spots: Spot[];
  desks: Spot[];
  names: Name[];
  mine?: Box & { name: [number, number] };
};

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
function boxOf(pic: Picture, frame: Frame, tag: string): Box | null {
  let [x0, y0, x1, y1] = [Infinity, Infinity, -1, -1];
  for (let i = 0; i < pic.id.length; i++) {
    if (frame.meta[pic.id[i]]?.tag !== tag) continue;
    const [x, y] = [i % pic.w, Math.floor(i / pic.w)];
    [x0, y0, x1, y1] = [Math.min(x0, x), Math.min(y0, y), Math.max(x1, x), Math.max(y1, y)];
  }
  return x1 < 0 ? null : { x: x0, y: y0, w: x1 - x0 + 1, h: y1 - y0 + 1 };
}

/** The room's pixels inside `box` that `covers` picks, as a small image; none when nothing is picked. */
function frontOf(pic: Picture, box: Box, covers: (i: number) => boolean): Figure['front'] {
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
  if (!any) return undefined;
  const canvas = document.createElement('canvas');
  [canvas.width, canvas.height] = [img.width, img.height];
  canvas.getContext('2d')?.putImageData(img, 0, 0);
  return { url: canvas.toDataURL(), x: x0, y: y0, w: img.width, h: img.height };
}

/** Where every placed person is drawn, back to front, and where each is for clicks and names; door shadows go into the picture. */
export function castOf(pic: Picture, frame: Frame, room: Room): Cast {
  const cast: Cast = { figures: [], bubbles: [], spots: [], desks: [], names: [] };
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

  for (const place of [...room.places].sort((a, b) => a.z - b.z)) {
    const p = place.person;
    const row = rows.get(p.key) ?? 0;
    const draw = (pose: Pose, cx: number, bottom: number, covers: (i: number) => boolean) => {
      const box = boxFor(pose, cx, bottom);
      const tall = TALL[pose][row] * SCALE;
      const figure: Figure = { person: p, col: POSES.indexOf(pose), row, box, front: frontOf(pic, box, covers) };
      cast.figures.push(figure);
      return { x: cx - box.w / 2, y: bottom - tall, w: box.w, h: tall };
    };
    if (place.how === 'desk' && place.chair) {
      const chair = boxOf(pic, frame, place.chair);
      if (!chair) continue;
      // Head and shoulders show over the chair's back: the head's top 22 px above the chair's top, whatever the figure's height.
      const tall = TALL.sit_back[row] * SCALE;
      // Only the sitter's own chair covers it, and anything nearer than that chair: the desk and screen behind it never do.
      const tag = place.chair;
      let near = Infinity;
      for (let i = 0; i < pic.id.length; i++) if (frame.meta[pic.id[i]]?.tag === tag) near = Math.min(near, pic.depth[i]);
      const seen = draw(
        'sit_back',
        chair.x + chair.w / 2,
        chair.y - 22 + tall,
        (i) => frame.meta[pic.id[i]]?.tag === tag || pic.depth[i] < near - 0.05,
      );
      if (p.activity === 'you') cast.bubbles.push({ x: seen.x + seen.w - 8, y: seen.y - 6, w: BUBBLE_PX, h: BUBBLE_PX });
      cast.names.push({ person: p, x: chair.x + chair.w / 2, y: chair.y + chair.h + 1 });
      continue;
    }
    const sofa = place.how === 'sofa';
    const [fx, fy] = frame.toPixel(place.x, sofa ? 0.1 : 0, place.z);
    if (!sofa) shadowAt(pic, frame, place.x, place.z);
    // A sitter sinks into the sofa: only what stands well in front of it hides it.
    const depth = depthAt(place.x, place.z);
    const seen = draw(sofa ? 'sit_mug' : 'walk_down', fx, fy, (i) => pic.depth[i] < depth - (sofa ? 0.6 : 0.08));
    cast.spots.push({ person: p, ...seen });
    cast.names.push({ person: p, x: seen.x + seen.w / 2, y: seen.y + seen.h + 1 });
  }
  for (const person of room.people) {
    const desk = boxOf(pic, frame, `desk:${person.key}`);
    if (desk) cast.desks.push({ ...desk, person });
  }
  const mine = boxOf(pic, frame, MY_DESK);
  if (mine) cast.mine = { ...mine, name: [mine.x + mine.w / 2, mine.y + mine.h + 1] };
  return cast;
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
