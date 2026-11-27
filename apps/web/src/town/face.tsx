// A person's face for a tab: the head of their front-facing figure on the town's people sheet (people.ts), as their office draws them.
import { PEOPLE_SHEET, SHEET } from '#src/town/people.ts';

/** The sheet's column of the figure walking toward the viewer, and the square round its head in a cell. */
const FRONT = 2;
const HEAD = { x: 32, y: 6, size: 76 };
/** The square round the whole head, hair and chin, of every figure: for a face drawn large enough to show it. */
const WHOLE = { x: 12, y: 4, size: 116 };

/** `whole`: the whole head, not only the face a tab's small mark has room for. */
export function Face({ row, size = 16, whole = false }: { row: number; size?: number; whole?: boolean }) {
  const [cw, ch] = SHEET.cell;
  const head = whole ? WHOLE : HEAD;
  return (
    <svg
      width={size}
      height={size}
      viewBox={`${FRONT * cw + head.x} ${row * ch + head.y} ${head.size} ${head.size}`}
      className="shrink-0 rounded-sm"
      aria-hidden
    >
      <image href={PEOPLE_SHEET} width={SHEET.size[0]} height={SHEET.size[1]} />
    </svg>
  );
}
