import { describe, expect, it } from 'vitest';
import { clickPick, dragged, noPick } from '#src/ui/tabPick.ts';

const ids = ['a', 'b', 'c', 'd', 'e'];
const plain = { add: false, range: false };
const add = { add: true, range: false };
const range = { add: false, range: true };

describe('tab pick', () => {
  it('Cmd+click adds and drops one, starting from the tab shown, and shows nothing new', () => {
    let p = clickPick(noPick, ids, 'a', 'c', add);
    expect(p).toEqual({ picked: ['a', 'c'], anchor: 'c', show: false });
    p = clickPick(p, ids, 'a', 'e', add);
    expect(p.picked).toEqual(['a', 'c', 'e']);
    expect(clickPick(p, ids, 'a', 'c', add).picked).toEqual(['a', 'e']);
  });

  it('Shift+click picks the run from the last clicked tab, either way, and shows the clicked one', () => {
    const p = clickPick(clickPick(noPick, ids, 'a', 'b', plain), ids, 'b', 'd', range);
    expect(p).toEqual({ picked: ['b', 'c', 'd'], anchor: 'b', show: true });
    expect(clickPick(p, ids, 'd', 'a', range).picked).toEqual(['a', 'b']);
  });

  it('a plain click drops the pick; a drag carries the pick only from a picked tab', () => {
    const p = clickPick(noPick, ids, 'a', 'c', range);
    expect(dragged(p, 'b')).toEqual(['a', 'b', 'c']);
    expect(dragged(p, 'e')).toEqual(['e']);
    expect(clickPick(p, ids, 'c', 'e', plain).picked).toEqual([]);
  });
});
