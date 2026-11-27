import { describe, expect, it } from 'vitest';
import { pushEscapeLayer } from '#src/ui/useEscape.ts';

const esc = () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));

describe('Esc layers', () => {
  it('closes the newest layer first, one per press', () => {
    const closed: string[] = [];
    let popViewer = () => {};
    let popNote = () => {};
    popViewer = pushEscapeLayer({ take: () => (closed.push('viewer'), popViewer()) });
    popNote = pushEscapeLayer({ take: () => (closed.push('note'), popNote()) });
    esc();
    expect(closed).toEqual(['note']);
    esc();
    expect(closed).toEqual(['note', 'viewer']);
    esc();
    expect(closed).toEqual(['note', 'viewer']);
  });

  it('leaves Esc to a focused field when the layer has no element', () => {
    let closed = 0;
    const pop = pushEscapeLayer({ take: () => closed++ });
    const input = document.body.appendChild(document.createElement('input'));
    input.addEventListener('keydown', (e) => e.preventDefault());
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
    expect(closed).toBe(0);
    pop();
    input.remove();
  });
});
