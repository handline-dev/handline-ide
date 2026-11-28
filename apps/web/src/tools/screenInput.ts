// A person's pointer, wheel and keys on a screen view, as screen.input events in the frame's units.
import type { ScreenInput } from '@handline/client';

/** Where a frame of `w`x`h` is drawn, fitted whole inside a box of `bw`x`bh`: its offset and scale. */
export function fitOf(w: number, h: number, bw: number, bh: number) {
  const scale = w && h ? Math.min(bw / w, bh / h) : 1;
  return { scale, left: (bw - w * scale) / 2, top: (bh - h * scale) / 2 };
}

/** A point in the box (its own pixels) in the frame's units, or null outside the frame; `clamp` brings it to the nearest edge instead. */
export function toFrame(px: number, py: number, w: number, h: number, bw: number, bh: number, clamp = false) {
  const f = fitOf(w, h, bw, bh);
  const x = (px - f.left) / f.scale;
  const y = (py - f.top) / f.scale;
  if (clamp) return { x: Math.round(Math.min(w, Math.max(0, x))), y: Math.round(Math.min(h, Math.max(0, y))) };
  return x < 0 || y < 0 || x > w || y > h ? null : { x: Math.round(x), y: Math.round(y) };
}

/** Alt 1, Ctrl 2, Meta 4, Shift 8. */
export const modifiersOf = (e: { altKey: boolean; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean }) =>
  (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0);

const BUTTON = ['left', 'middle', 'right'] as const;

export const mouseOf = (
  action: 'move' | 'down' | 'up',
  at: { x: number; y: number },
  e: { button: number; detail: number },
): ScreenInput =>
  action === 'move'
    ? { kind: 'mouse', action, ...at }
    : { kind: 'mouse', action, ...at, button: BUTTON[e.button] ?? 'left', clicks: Math.max(1, e.detail) };

/** A key down or up; a printable key's down carries the text it types (Enter as \r). */
export function keyOf(
  action: 'down' | 'up',
  e: { key: string; code: string; altKey: boolean; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean },
): ScreenInput {
  const modifiers = modifiersOf(e);
  const types = action === 'down' && !e.ctrlKey && !e.metaKey && (e.key.length === 1 || e.key === 'Enter');
  return { kind: 'key', action, key: e.key, code: e.code, modifiers, ...(types && { text: e.key === 'Enter' ? '\r' : e.key }) };
}
