import { describe, expect, it } from 'vitest';
import { ChangeSet, Text } from '@codemirror/state';
import { diffChanges } from '#src/editor/diff.ts';

const apply = (a: string, b: string) => diffChanges(a, b).apply(Text.of(a.split('\n'))).toString();

describe('diffChanges', () => {
  const cases: [string, string][] = [
    ['', ''],
    ['a', 'b'],
    ['a\nb\nc', 'a\nc'],
    ['a\nb\nc', 'a\nx\nb\nc'],
    ['a\nb', 'a\nb\nc'],
    ['a\nb\nc', 'a\nb'],
    ['x\na\nb\nc\ny', 'x\nA\nb\nC\ny'],
    ['', 'new\ntext'],
    ['one\ntwo', ''],
  ];
  it.each(cases)('turns %j into %j', (a, b) => expect(apply(a, b)).toBe(b));

  it('keeps a local edit between two far edits', () => {
    const base = 'one\ntwo\nthree\nfour\nfive';
    const far = diffChanges(base, 'ONE\ntwo\nthree\nfour\nFIVE');
    const mine = ChangeSet.of({ from: base.indexOf('three'), insert: 'my ' }, base.length);
    const out = mine.apply(Text.of(base.split('\n')));
    const laid = far.map(mine);
    expect(laid.apply(out).toString()).toBe('ONE\ntwo\nmy three\nfour\nFIVE');
  });
});
