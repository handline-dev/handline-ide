import { describe, expect, it } from 'vitest';
import { tabKeyTarget } from '#src/layout/tabKeys.ts';

const pane = { tabs: [{ id: 'a' }, { id: 'b' }, { id: 'c' }], active: 'a' };
const k = (o: Partial<KeyboardEvent>) =>
  ({ key: '', code: '', ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...o }) as KeyboardEvent;

describe('tabKeyTarget', () => {
  it('steps round the ends', () => {
    expect(tabKeyTarget(k({ key: 'Tab', ctrlKey: true }), pane, true)).toBe('b');
    expect(tabKeyTarget(k({ key: 'Tab', ctrlKey: true, shiftKey: true }), pane, true)).toBe('c');
    expect(tabKeyTarget(k({ key: 'PageUp', ctrlKey: true }), pane, false)).toBe('c');
    expect(tabKeyTarget(k({ code: 'BracketRight', metaKey: true, shiftKey: true }), pane, true)).toBe('b');
    expect(tabKeyTarget(k({ code: 'BracketRight', metaKey: true, shiftKey: true }), pane, false)).toBeNull();
  });
  it('goes by number, 9 to the last', () => {
    expect(tabKeyTarget(k({ key: '2', metaKey: true }), pane, true)).toBe('b');
    expect(tabKeyTarget(k({ key: '2', ctrlKey: true }), pane, false)).toBe('b');
    expect(tabKeyTarget(k({ key: '2', ctrlKey: true }), pane, true)).toBeNull();
    expect(tabKeyTarget(k({ key: '5', metaKey: true }), pane, true)).toBeNull();
    expect(tabKeyTarget(k({ key: '9', metaKey: true }), pane, true)).toBe('c');
  });
  it('also steps by PageDown and [', () => {
    expect(tabKeyTarget(k({ key: 'PageDown', ctrlKey: true }), pane, false)).toBe('b');
    expect(tabKeyTarget(k({ code: 'BracketLeft', metaKey: true, shiftKey: true }), pane, true)).toBe('c');
  });
  it('leaves other keys alone', () => {
    expect(tabKeyTarget(k({ key: 'Tab', ctrlKey: true }), { tabs: [], active: '' }, true)).toBeNull();
    expect(tabKeyTarget(k({ key: '2', metaKey: true, shiftKey: true }), pane, true)).toBeNull();
    expect(tabKeyTarget(k({ key: 'Tab' }), pane, true)).toBeNull();
    expect(tabKeyTarget(k({ key: 'Tab', ctrlKey: true, altKey: true }), pane, true)).toBeNull();
  });
});
