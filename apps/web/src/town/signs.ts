// The buildings' signs as the city shows them at a scale (Signs.tsx draws them), in the picture's pixels. In the close-up a name is on
// the sign drawn on its building. Zoomed out, the text keeps about its size on screen, so a sign becomes a plate larger than the drawn
// one, on the edge of the building's roof where it hides neither windows nor door: the name with a line under it where the whole row
// has room for that, the names alone where two plates of the row would meet, and where even the names would, each sign kept to its own
// building's width: its badges over its plate, its name cut short.

/** A box in picture pixels. */
type Box = { x: number; y: number; w: number; h: number };
/** How wide a text is, for each pixel of its height. */
export type Measure = (text: string) => number;

export type Post = {
  key: string;
  /** The sign drawn on the building. */
  box: Box;
  /** The building's width: what its plate may take when its neighbours' plates are in the way. */
  room: number;
  /** Which row of buildings it stands in: only signs of one row can meet. */
  row: number;
  /** Where the top of the building's front is in the picture: zoomed out its plate rests there, on the roof's edge, clear of its
   *  windows and its door. None for what has no roof, whose plate stays over its drawn sign. */
  roof?: number;
  name: string;
  /** What it reads where its name would have to be cut short, when that says more than the cut name. */
  short?: string;
  sub: string;
  /** Its badges' texts, in the order drawn. */
  marks: string[];
};
/** A sign to draw: its plate (`drawn`: it is the building's own, nothing to paint), its lines and their text heights, and where its badges
 *  start and how much larger than in the close-up they are drawn. */
export type Plate = Box & {
  key: string;
  drawn: boolean;
  name: string;
  sub: string | null;
  font: number;
  subFont: number;
  /** The middle of the name's line. */
  line: number;
  marks: { x: number; y: number; scale: number };
};

/** A name's height on the sign drawn on a building. */
const DRAWN = 8.5;

/** A sign's text height in picture pixels: as drawn in the close-up, and from 8.5 to 10.5 pixels on screen however far out. */
export const signFont = (scale: number) => (scale >= 1 ? DRAWN : (DRAWN + Math.min(2, (1 - scale) * 5)) / scale);
/** How much larger than in the close-up a badge is drawn, so it never shows under 0.85 of its size on screen. */
export const badgeScale = (scale: number) => Math.max(1, 0.85 / scale);
/** A badge's width in the close-up. */
export const badgeWidth = (text: string) => Math.max(16, text.length * 6.5 + 8);
/** The founder's name tag zoomed out: 10 pixels tall on screen. */
export const tagFont = (scale: number) => 10 / scale;

const rough: Measure = (text) => text.length * 0.6;

/** `name` whole when it is no wider than `room` at `font`, else its start and an ellipsis; never less than a letter. */
export function cut(name: string, room: number, font: number, measure: Measure = rough): string {
  if (measure(name) * font <= room) return name;
  for (let n = name.length - 1; n > 1; n--) {
    const short = `${name.slice(0, n).trimEnd()}…`;
    if (measure(short) * font <= room) return short;
  }
  return `${name.slice(0, 1)}…`;
}

/** Every sign at `scale`. Zoomed out, a plate and its badges together sit over the middle of the building. */
export function platesOf(posts: Post[], scale: number, measure: Measure = rough): Plate[] {
  if (scale >= 1)
    return posts.map(({ key, box, name }) => ({
      ...box,
      key,
      drawn: true,
      name,
      sub: null,
      font: DRAWN,
      subFont: DRAWN,
      line: box.y + box.h / 2,
      marks: { x: box.x + box.w + 3, y: box.y + box.h / 2, scale: 1 },
    }));
  const font = signFont(scale);
  const subFont = font * 0.85;
  const [pad, row, gap, k] = [font * 0.55, font * 1.5, 2 / scale, badgeScale(scale)];
  const sized = posts.map((p) => {
    const marks = p.marks.length ? k * (1 + p.marks.reduce((n, m) => n + badgeWidth(m) + 2, 0)) : 0;
    const mid = p.box.x + p.box.w / 2;
    /** A plate `w` wide and `h` tall with its badges beside it, and the span the two take along the row: resting on the roof's front
     *  edge where the building has one, else over the middle of the drawn sign. */
    const place = (w: number, h: number) => {
      const x = mid - (w + marks) / 2;
      const y = p.roof === undefined ? p.box.y + p.box.h / 2 - row / 2 : p.roof - gap - h;
      return { plate: { x, y, w, h }, span: [x, x + w + marks] as const };
    };
    const bare = place(Math.max(p.box.w, measure(p.name) * font + 2 * pad), row);
    const full = p.sub ? place(Math.max(bare.plate.w, measure(p.sub) * subFont + 2 * pad), row + subFont * 1.35) : bare;
    return { p, marks, mid, bare, full };
  });
  type Sized = (typeof sized)[number];
  /** Whether two signs of one row would meet, each as wide as `of` says. */
  const meet = (a: Sized, b: Sized, of: (s: Sized) => { span: readonly [number, number] }) =>
    a !== b && a.p.row === b.p.row && of(a).span[0] < of(b).span[1] + gap && of(b).span[0] < of(a).span[1] + gap;
  // A row shows the lines under its names only when every one of them has room.
  const crowded = new Set(sized.filter((a) => sized.some((b) => meet(a, b, (s) => s.full))).map((a) => a.p.row));
  return sized.map((s) => {
    const whole = !!s.p.sub && !crowded.has(s.p.row);
    // Kept to its own building: when the name and the badges do not fit it side by side, the badges go over the plate.
    const wide = s.bare.span[1] - s.bare.span[0] + gap > s.p.room;
    const tight = !whole && wide && sized.some((o) => meet(s, o, (x) => x.bare));
    const room = s.p.room - 2 * pad - gap;
    const fits = measure(s.p.name) * font <= room;
    const name = tight ? cut(fits ? s.p.name : (s.p.short ?? s.p.name), room, font, measure) : s.p.name;
    const mid = s.mid;
    const w = measure(name) * font + 2 * pad;
    const plate = whole ? s.full.plate : tight ? { ...s.bare.plate, x: mid - w / 2, w } : s.bare.plate;
    const line = plate.y + row / 2;
    const over = { x: mid - (s.marks - 3 * k) / 2, y: plate.y - 8 * k - gap / 2, scale: k };
    return {
      ...plate,
      key: s.p.key,
      drawn: false,
      name,
      sub: whole ? s.p.sub : null,
      font,
      subFont,
      line,
      marks: tight ? over : { x: plate.x + plate.w + k, y: line, scale: k },
    };
  });
}
