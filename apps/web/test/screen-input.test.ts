import { describe, expect, it } from 'vitest';
import { fitOf, keyOf, mouseOf, toFrame } from '#src/tools/screenInput.ts';

const KEY = { altKey: false, ctrlKey: false, metaKey: false, shiftKey: false };

describe('screen input', () => {
  it('fits the frame whole and centred in the pane', () => {
    expect(fitOf(1000, 500, 500, 500)).toEqual({ scale: 0.5, left: 0, top: 125 });
  });
  it('maps a point in the pane to the frame, and none outside it', () => {
    expect(toFrame(250, 250, 1000, 500, 500, 500)).toEqual({ x: 500, y: 250 });
    expect(toFrame(250, 10, 1000, 500, 500, 500)).toBeNull();
  });
  it('sends a press with its button and count, a move without', () => {
    expect(mouseOf('down', { x: 1, y: 2 }, { button: 2, detail: 2 })).toEqual({
      kind: 'mouse',
      action: 'down',
      x: 1,
      y: 2,
      button: 'right',
      clicks: 2,
    });
    expect(mouseOf('move', { x: 1, y: 2 }, { button: 0, detail: 0 })).toEqual({ kind: 'mouse', action: 'move', x: 1, y: 2 });
  });
  it('types a printable key and Enter, not a shortcut', () => {
    expect(keyOf('down', { ...KEY, key: 'a', code: 'KeyA' }).text).toBe('a');
    expect(keyOf('down', { ...KEY, key: 'Enter', code: 'Enter' }).text).toBe('\r');
    expect(keyOf('down', { ...KEY, metaKey: true, key: 'c', code: 'KeyC' })).toEqual({
      kind: 'key',
      action: 'down',
      key: 'c',
      code: 'KeyC',
      modifiers: 4,
    });
    expect(keyOf('up', { ...KEY, key: 'a', code: 'KeyA' }).text).toBeUndefined();
  });
});
