// The people in the 3D room: flat figures from public/town/office.png, pasted pixel for pixel (the room is drawn at their scale, so they
// are never resized). One at its desk sits with its back to us, the chair laid back over it so only head and shoulders show; an idle one
// sits on a sofa; one starting stands at the door on a small shadow. Each person is one of the sheet's figures, teammates different ones
// while there are enough. A person hides behind whatever stands nearer than it.
import type { Person } from '#src/town/model.ts';
import type { Room } from '#src/town/room.ts';
import type { Frame } from '#src/town/scene3d.ts';
import type { Picture } from '#src/town/shade.ts';
import { FIGURE, FIGURES, SPRITES } from '#src/town/sprites.ts';

export const OFFICE_SHEET = `${import.meta.env.BASE_URL}town/office.png`;

/** What a click opens an agent's chat from, in picture pixels: its figure, or its desk with its chair. */
export type Spot = { person: Person; x: number; y: number; w: number; h: number };
/** A person's name, centred on x with its top at y. */
export type Name = { person: Person; x: number; y: number };
export type Cast = { spots: Spot[]; desks: Spot[]; names: Name[] };

const hashOf = (key: string) => [...key].reduce((n, ch) => (n * 31 + ch.charCodeAt(0)) >>> 0, 7);

/** A figure per person from its key, so it keeps it: the first from there on that no teammate has yet. */
export function figuresOf(people: Person[]): Map<string, number> {
  const taken = new Set<number>();
  return new Map(
    people.map((p) => {
      const start = hashOf(p.key) % FIGURES;
      const figure = Array.from({ length: FIGURES }, (_, i) => (start + i) % FIGURES).find((f) => !taken.has(f)) ?? start;
      taken.add(figure);
      return [p.key, figure];
    }),
  );
}

/** The pixels a room object covers, by its tag, as a box. */
function boxOf(pic: Picture, frame: Frame, tag: string): Spot | null {
  let [x0, y0, x1, y1] = [Infinity, Infinity, -1, -1];
  for (let i = 0; i < pic.id.length; i++) {
    if (frame.meta[pic.id[i]]?.tag !== tag) continue;
    const [x, y] = [i % pic.w, Math.floor(i / pic.w)];
    [x0, y0, x1, y1] = [Math.min(x0, x), Math.min(y0, y), Math.max(x1, x), Math.max(y1, y)];
  }
  return x1 < 0 ? null : ({ x: x0, y: y0, w: x1 - x0 + 1, h: y1 - y0 + 1 } as Spot);
}

/** Puts every placed person into the picture, in place, back to front; returns where each is for clicks and names. */
export function castInto(pic: Picture, frame: Frame, room: Room, sheet: ImageData): Cast {
  const cast: Cast = { spots: [], desks: [], names: [] };
  const figures = figuresOf(room.people);
  const before = pic.rgba.slice();
  /** Pastes a sprite with its bottom centre at (cx, bottom), skipping pixels where the room is nearer than `depth`. */
  const paste = (sprite: keyof typeof SPRITES, figure: number, cx: number, bottom: number, depth: number, slack = 0.08) => {
    const [sx, sy0, sw, sh] = SPRITES[sprite];
    const sy = sy0 + figure * FIGURE;
    const [x0, y0] = [Math.round(cx - sw / 2), Math.round(bottom - sh)];
    for (let y = 0; y < sh; y++) {
      for (let x = 0; x < sw; x++) {
        const s = ((sy + y) * sheet.width + sx + x) * 4;
        const [px, py] = [x0 + x, y0 + y];
        if (sheet.data[s + 3] < 128 || px < 0 || py < 0 || px >= pic.w || py >= pic.h) continue;
        const i = py * pic.w + px;
        if (pic.depth[i] < depth - slack) continue;
        pic.rgba.set([sheet.data[s], sheet.data[s + 1], sheet.data[s + 2], 255], i * 4);
      }
    }
    return { x: x0, y: y0, w: sw, h: sh };
  };
  const depthAt = (x: number, z: number) => {
    const [px, py] = frame.toPixel(x, 0.5, z).map(Math.round);
    return pic.depth[py * pic.w + px] ?? 1e9;
  };

  // Nearest last, so a nearer person covers a farther one.
  for (const place of [...room.places].sort((a, b) => a.z - b.z)) {
    const p = place.person;
    const figure = figures.get(p.key) ?? 0;
    if (place.how === 'desk' && place.chair) {
      const chair = boxOf(pic, frame, place.chair);
      if (!chair) continue;
      const spot = paste('sit_back', figure, chair.x + chair.w / 2, chair.y + 14, depthAt(place.x, place.z - 0.3));
      // The chair's back covers the sitter's back.
      for (let i = 0; i < pic.id.length; i++)
        if (frame.meta[pic.id[i]]?.tag === place.chair) pic.rgba.set(before.subarray(i * 4, i * 4 + 4), i * 4);
      if (p.activity === 'you') paste('bubble', 0, spot.x + spot.w - 2, spot.y + 12, 0);
      cast.names.push({ person: p, x: chair.x + chair.w / 2, y: chair.y + chair.h + 1 });
      continue;
    }
    const [fx, fy] = frame.toPixel(place.x, place.how === 'sofa' ? 0.1 : 0, place.z);
    if (place.how === 'door') shadowAt(pic, frame, place.x, place.z);
    // A sitter sinks into the sofa: only what stands well in front of it hides it.
    const spot = paste(
      place.how === 'sofa' ? 'sit_sofa' : 'walk_down',
      figure,
      fx,
      Math.round(fy),
      depthAt(place.x, place.z),
      place.how === 'sofa' ? 0.6 : 0.08,
    );
    cast.spots.push({ person: p, ...spot });
    cast.names.push({ person: p, x: spot.x + spot.w / 2, y: spot.y + spot.h + 1 });
  }
  for (const person of room.people) {
    const desk = boxOf(pic, frame, `desk:${person.key}`);
    if (desk) cast.desks.push({ ...desk, person });
  }
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
