// The names under an office's desks, laid out so none covers another: a bench's desks stand closer than a long name is wide, so a name
// that would run into the one before it drops to a second line, and one wider than two desks is cut short with an ellipsis.

/** A label centred on x with its top at y, `text` as it is written. */
export type Placed<T> = { item: T; x: number; y: number; text: string };

/** How much of a name's height its second line drops. */
const LINE = 1.15;
/** The space kept between two names on a line, as a share of the font. */
const GAP = 0.5;

/** Lays out labels of `font` size, each centred at its x under its seat; `width` measures a text. Labels whose tops are within a font of
 *  each other share a row. */
export function lineUp<T>(labels: { item: T; x: number; y: number; text: string }[], font: number, width: (text: string) => number) {
  const out: Placed<T>[] = [];
  const rows: (typeof labels)[] = [];
  for (const l of [...labels].sort((a, b) => a.y - b.y)) {
    const row = rows.find((r) => Math.abs(r[0].y - l.y) < font);
    if (row) row.push(l);
    else rows.push([l]);
  }
  const gap = font * GAP;
  for (const row of rows) {
    row.sort((a, b) => a.x - b.x);
    // Where each line's last name ends.
    const ends = [-Infinity, -Infinity];
    for (const [i, l] of row.entries()) {
      // No wider than reaches halfway to the seat after next, so two lines always have room for every name.
      const near = Math.min(l.x - (row[i - 1]?.x ?? -Infinity), (row[i + 1]?.x ?? Infinity) - l.x);
      const whole = cut(l.text, 2 * near - gap, width);
      const w = width(whole);
      const line = ends.findIndex((end) => end + gap <= l.x - w / 2);
      if (line >= 0) {
        ends[line] = l.x + w / 2;
        out.push({ ...l, y: l.y + line * font * LINE, text: whole });
        continue;
      }
      // Seats closer on one side than the other: the line that ends first takes it, cut to fit after its last name.
      const k = ends[0] <= ends[1] ? 0 : 1;
      const text = cut(whole, 2 * (l.x - ends[k] - gap), width);
      ends[k] = l.x + width(text) / 2;
      out.push({ ...l, y: l.y + k * font * LINE, text });
    }
  }
  return out;
}

/** The longest start of a text, with an ellipsis, no wider than `room`; at least its first letter. */
export function cut(text: string, room: number, width: (text: string) => number): string {
  if (width(text) <= room) return text;
  for (let n = text.length - 1; n > 1; n--) if (width(`${text.slice(0, n)}…`) <= room) return `${text.slice(0, n)}…`;
  return `${text.slice(0, 1)}…`;
}
