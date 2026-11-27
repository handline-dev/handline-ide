// The city's zoom, as sums (camera.tsx keeps the view): the scales a pane allows, from the whole town in view to the close-up, a level
// between the two, and where the picture sits in the pane: a point kept still under the pointer, the founder kept in view, and no edge
// of the picture shown past.

export type Pair = readonly [number, number];
/** The scales a picture may be shown at: `min` shows the whole of it in the pane, `max` is the close-up. */
export type Range = { min: number; max: number };

/** How near either end a zoom snaps to it, as a share of the way. */
const SNAP = 0.02;
/** How near the pane's edge the founder may stand with the whole town in view, in pane pixels. */
const EDGE = 48;
/** How many presses of + take the whole town to the close-up. */
const STEPS = 3;

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

/** The scales for a picture of `size` in `pane`: the close-up is the picture's own pixels, or larger when the whole fits larger (then
 *  there is nothing to zoom). */
export function rangeOf([w, h]: Pair, [pw, ph]: Pair): Range {
  const fit = Math.min(pw / w, ph / h);
  if (!(fit > 0) || !Number.isFinite(fit)) return { min: 1, max: 1 };
  return { min: fit, max: Math.max(1, fit) };
}

export const zooms = ({ min, max }: Range) => max > min;

/** The scale at a level: 0 is the whole town, 1 the close-up (each exactly its end, with no rounding), even steps of level being even
 *  steps of zoom. */
export const scaleAt = (level: number, { min, max }: Range) => (level >= 1 ? max : level <= 0 ? min : min * (max / min) ** level);

/** The level a scale is at, kept between the two ends; the close-up when there is nothing to zoom. */
export const levelOf = (scale: number, r: Range) => (zooms(r) ? clamp(Math.log(scale / r.min) / Math.log(r.max / r.min), 0, 1) : 1);

/** The level after zooming by `factor` (over 1 is closer), resting on the end it is going to when nearly there. Never on the end it is
 *  leaving: the small steps of a pinch or a slow wheel add up from there. */
export function zoomed(level: number, factor: number, r: Range): number {
  const next = levelOf(scaleAt(level, r) * factor, r);
  if (factor < 1 && next < SNAP) return 0;
  if (factor > 1 && next > 1 - SNAP) return 1;
  return next;
}

/** The level a press of + (`dir` 1) or - (`dir` -1) goes to: the next of a few even stops between the two ends. */
export function stepped(level: number, dir: 1 | -1): number {
  const at = level * STEPS;
  const next = dir > 0 ? Math.floor(at + 1e-6) + 1 : Math.ceil(at - 1e-6) - 1;
  return clamp(next / STEPS, 0, 1);
}

/** The level `k` of the way (0 to 1) through an eased step from `from` to `to`: fast at first, slowing into where it is going. */
export const eased = (from: number, to: number, k: number) => from + (to - from) * (1 - (1 - clamp(k, 0, 1)) ** 3);

/** One axis of the picture in the pane: in the middle when it fits, else never showing past either edge. */
const held = (size: number, pane: number, o: number) => (size <= pane ? (pane - size) / 2 : clamp(o, pane - size, 0));

/** The picture's top left in the pane, moved the least so the picture fills the pane, or sits in its middle where it is smaller. */
export const inPane = (o: Pair, [w, h]: Pair, [pw, ph]: Pair, scale: number): Pair => [
  held(w * scale, pw, o[0]),
  held(h * scale, ph, o[1]),
];

/** The top left that keeps the picture's point under pane point `p` still as the scale goes from `from` to `to`. */
export const keepPoint = (o: Pair, p: Pair, from: number, to: number): Pair => [
  p[0] - ((p[0] - o[0]) * to) / from,
  p[1] - ((p[1] - o[1]) * to) / from,
];

/** How far from the pane's middle the founder may stand along an axis: nowhere in the close-up, where the view stays on it, out to near
 *  the edge with the whole town in view; most of that already a little way out, so a zoom there keeps what is under the pointer. */
export const reachOf = (level: number, pane: number) => (1 - clamp(level, 0, 1) ** 2) * Math.max(0, pane / 2 - EDGE);

/** One axis moved the least so a point drawn at `at` (in scaled picture pixels) shows within `reach` of the pane's middle. */
function near(o: number, at: number, pane: number, reach: number) {
  const off = pane / 2 - (o + at);
  return o + off - clamp(off, -reach, reach);
}

/** Where the picture rests at `level`, starting from top left `o`: the founder's `spot` (in picture pixels, if it is there) brought within
 *  reach of the middle, and the picture kept in the pane. */
export function settle(o: Pair, spot: Pair | null, level: number, size: Pair, pane: Pair): Pair {
  const scale = scaleAt(level, rangeOf(size, pane));
  const axis = (i: 0 | 1) => (spot ? near(o[i], spot[i] * scale, pane[i], reachOf(level, pane[i])) : o[i]);
  return inPane([axis(0), axis(1)], size, pane, scale);
}
