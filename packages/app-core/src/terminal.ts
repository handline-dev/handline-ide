// What every app's terminal shares: the resize debounce, the colours past the theme's 16 and the phone's fit of a terminal to its box. Pure.

/** How long a terminal waits after its last resize before it tells the daemon its new size. */
export const RESIZE_MS = 150;

const CUBE = [0, 95, 135, 175, 215, 255];
const hex = (n: number) => n.toString(16).padStart(2, '0');

/** Palette index 16-255 as `#rrggbb`: the xterm 6×6×6 cube, then 24 greys. Undefined for any other index; 0-15 are each app's theme. */
export function xtermColor(c: number): string | undefined {
  if (!Number.isInteger(c) || c < 16 || c > 255) return undefined;
  if (c < 232) {
    const i = c - 16;
    return `#${hex(CUBE[Math.floor(i / 36)]!)}${hex(CUBE[Math.floor(i / 6) % 6]!)}${hex(CUBE[i % 6]!)}`;
  }
  const g = 8 + (c - 232) * 10;
  return `#${hex(g)}${hex(g)}${hex(g)}`;
}

/** The daemon never sizes a terminal under this. */
export const MIN_COLS = 80;
export const MIN_ROWS = 24;
export const CELL_W = 0.6; // a monospace cell's width in em
export const LINE_H = 1.25; // line height in em
export const MAX_FONT = 11;

/** What the phone shows of the terminal: a font that fits `cols` across `width`, and the grid that font gives. */
export function fitOf(width: number, height: number): { font: number; cols: number; rows: number } {
  const font = Math.min(MAX_FONT, width / (MIN_COLS * CELL_W));
  return {
    font,
    cols: Math.max(MIN_COLS, Math.floor(width / (font * CELL_W))),
    rows: Math.max(MIN_ROWS, Math.floor(height / (font * LINE_H))),
  };
}
