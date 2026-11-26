// A person's face for a tab: the head of their front-facing figure on the town's people sheet (people.ts), as their office draws them.
import { PEOPLE_SHEET, SHEET } from '#src/town/people.ts';

/** The sheet's column of the figure walking toward the viewer, and the square round its head in a cell. */
const FRONT = 2;
const HEAD = { x: 32, y: 6, size: 76 };

export function Face({ row, size = 16 }: { row: number; size?: number }) {
  const [cw, ch] = SHEET.cell;
  return (
    <svg
      width={size}
      height={size}
      viewBox={`${FRONT * cw + HEAD.x} ${row * ch + HEAD.y} ${HEAD.size} ${HEAD.size}`}
      className="shrink-0 rounded-sm"
      aria-hidden
    >
      <image href={PEOPLE_SHEET} width={SHEET.size[0]} height={SHEET.size[1]} />
    </svg>
  );
}
