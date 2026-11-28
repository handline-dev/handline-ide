// A person's face for a tab: the head of their front-facing figure on the town's people sheet (people.ts), as their office draws them.
import { PEOPLE_SHEET, SHEET } from '#src/town/sheet.ts';

/** The sheet's column of the figure walking toward the viewer, and per person the square (x, y, side) round its face in a cell, brow to
 *  chin, so even a face under a big head of dark hair reads at a tab's size, and the square round the whole head, hair and chin, for a
 *  face drawn large enough to show it. Measured on the sheet. */
const FRONT = 2;
const HEADS: { whole: [number, number, number]; face: [number, number, number] }[] = [
  { whole: [23, 36, 99], face: [37, 64, 71] },
  { whole: [21, 36, 105], face: [35, 65, 76] },
  { whole: [27, 27, 93], face: [40, 53, 67] },
  { whole: [18, 15, 111], face: [33, 46, 80] },
  { whole: [12, 21, 123], face: [29, 55, 89] },
  { whole: [21, 30, 105], face: [35, 59, 76] },
  { whole: [27, 42, 93], face: [40, 68, 67] },
  { whole: [23, 42, 99], face: [37, 70, 71] },
  { whole: [27, 48, 93], face: [40, 74, 67] },
  { whole: [23, 21, 99], face: [37, 49, 71] },
  { whole: [21, 15, 105], face: [35, 44, 76] },
  { whole: [12, 3, 123], face: [29, 37, 89] },
  { whole: [18, 33, 111], face: [33, 64, 80] },
];

/** `whole`: the whole head, not only the face a tab's small mark has room for. */
export function Face({ row, size = 16, whole = false }: { row: number; size?: number; whole?: boolean }) {
  const [cw, ch] = SHEET.cell;
  const [x, y, side] = whole ? HEADS[row].whole : HEADS[row].face;
  return (
    <svg
      width={size}
      height={size}
      viewBox={`${FRONT * cw + x} ${row * ch + y} ${side} ${side}`}
      className="shrink-0 rounded-sm"
      aria-hidden
    >
      <image href={PEOPLE_SHEET} width={SHEET.size[0]} height={SHEET.size[1]} />
    </svg>
  );
}
