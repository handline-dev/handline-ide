import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  classifyGesture,
  initialMarks,
  keptMarks,
  markData,
  markedText,
  markStarts,
  marksReducer,
  marksToShapes,
  seedMarks,
  type MarksAction,
} from '#src/marks.ts';
import { exportAnnotated } from '#src/annotation-raster.ts';

const run = (...actions: MarksAction[]) => actions.reduce(marksReducer, initialMarks());
const gesture = (...ps: [number, number][]): MarksAction[] => [
  { type: 'down', x: ps[0]![0], y: ps[0]![1] },
  ...ps.slice(1, -1).map(([x, y]): MarksAction => ({ type: 'move', x, y })),
  { type: 'up', x: ps.at(-1)![0], y: ps.at(-1)![1] },
];
const circle = (cx: number, cy: number, r: number, n = 24): [number, number][] =>
  Array.from({ length: n + 1 }, (_, i) => [cx + r * Math.cos((2 * Math.PI * i) / n), cy + r * Math.sin((2 * Math.PI * i) / n)]);

test('a tap pins a circle round the point, a drag draws a box, a loop marks an area', () => {
  const tap = classifyGesture([{ x: 100, y: 50 }], 8, 16);
  assert.deepEqual([tap.kind, tap.rect], ['pin', { x: 84, y: 34, w: 32, h: 32 }]);
  const small = classifyGesture(
    [
      { x: 100, y: 50 },
      { x: 103, y: 52 },
    ],
    8,
    16,
  );
  assert.equal(small.kind, 'pin'); // inside the slop: a tap with a shaky finger
  const box = classifyGesture(
    [
      { x: 200, y: 120 },
      { x: 150, y: 90 },
      { x: 40, y: 20 },
    ],
    8,
    16,
  );
  assert.deepEqual([box.kind, box.rect], ['box', { x: 40, y: 20, w: 160, h: 100 }]); // dragged up and left: still a normal rect
  const loop = classifyGesture(
    circle(100, 100, 40).map(([x, y]) => ({ x, y })),
    8,
    16,
  );
  assert.equal(loop.kind, 'area');
  assert.deepEqual(loop.rect, { x: 60, y: 60, w: 80, h: 80 });
  assert.ok(loop.points!.length > 3 && loop.points!.length <= 64);
});

test('a drag along an edge is a box with some height, and a drag out and back is not a loop', () => {
  const line = classifyGesture(
    [
      { x: 10, y: 10 },
      { x: 110, y: 10 },
    ],
    8,
    16,
  );
  assert.deepEqual([line.kind, line.rect.h], ['box', 8]);
  const back = classifyGesture(
    [
      { x: 10, y: 10 },
      { x: 110, y: 12 },
      { x: 12, y: 11 },
    ],
    8,
    16,
  );
  assert.equal(back.kind, 'area'); // the path went out and came back: what is between is marked
});

test('a gesture adds a mark with empty words and opens its memo; the numbers count the marks in order', () => {
  const s = run(...gesture([10, 10], [10, 10]), ...gesture([50, 50], [120, 90]));
  assert.deepEqual(
    s.marks.map((m) => [m.id, m.kind, m.text]),
    [
      [1, 'pin', ''],
      [2, 'box', ''],
    ],
  );
  assert.equal(s.open, 2);
  assert.equal(s.draft, null);
});

test('memo words are set on the mark that is open; an empty memo is dropped, with the numbers closing up', () => {
  let s = run(...gesture([10, 10], [10, 10]), { type: 'memo', id: 1, text: 'Tab label is cut off' });
  s = [...gesture([50, 50], [120, 90]), { type: 'memo', id: 2, text: '   ' } as MarksAction].reduce(marksReducer, s);
  s = [...gesture([5, 80], [30, 99]), { type: 'memo', id: 3, text: 'too small' } as MarksAction].reduce(marksReducer, s);
  assert.equal(s.marks.length, 3);
  assert.deepEqual(
    keptMarks(s.marks).map((m) => m.id),
    [1, 3],
  );
  assert.deepEqual(
    markData(s.marks).map((d) => [d.n, d.text]),
    [
      [1, 'Tab label is cut off'],
      [2, 'too small'],
    ],
  );
});

test('Undo takes back the last mark, and always steps back one gesture; it does nothing with none', () => {
  let s = run(...gesture([10, 10], [10, 10]), ...gesture([50, 50], [120, 90]));
  s = marksReducer(s, { type: 'undo' });
  assert.deepEqual(
    s.marks.map((m) => m.id),
    [1],
  );
  assert.equal(s.open, null);
  s = marksReducer(marksReducer(s, { type: 'undo' }), { type: 'undo' });
  assert.equal(s.marks.length, 0);
  // a removal is a step too
  s = run(...gesture([10, 10], [10, 10]), { type: 'remove', id: 1 });
  assert.equal(s.marks.length, 0);
  assert.equal(marksReducer(s, { type: 'undo' }).marks.length, 1);
});

test('clear drops every mark and its undo steps in one action', () => {
  const s = run(...gesture([10, 10], [10, 10]), ...gesture([50, 50], [50, 50]), { type: 'clear' });
  assert.deepEqual([s.marks.length, s.past.length, s.open], [0, 0, null]);
});

test('a cancelled gesture leaves nothing, and the sizes follow the picture', () => {
  const s = run({ type: 'scale', slop: 20, radius: 40 }, { type: 'down', x: 5, y: 5 }, { type: 'move', x: 90, y: 90 }, { type: 'cancel' });
  assert.deepEqual([s.draft, s.marks.length], [null, 0]);
  const pin = run({ type: 'scale', slop: 20, radius: 40 }, ...gesture([100, 100], [110, 105]));
  assert.deepEqual([pin.marks[0]!.kind, pin.marks[0]!.rect.w], ['pin', 80]);
});

test('the marked picture draws only the kept marks, in red, and a ring leaves its inside alone', () => {
  const s = run(...gesture([40, 40], [40, 40]), { type: 'memo', id: 1, text: 'here' }, ...gesture([5, 5], [20, 20]));
  const shapes = marksToShapes(s.marks, 2);
  assert.equal(shapes.filter((x) => x.kind === 'text').length, 1); // one number: the empty memo's mark is left out
  const out = exportAnnotated({ width: 100, height: 100, data: new Uint8ClampedArray(100 * 100 * 4) }, shapes);
  const px = (x: number, y: number) => Array.from(out.data.slice((y * 100 + x) * 4, (y * 100 + x) * 4 + 4));
  assert.deepEqual(px(24, 40), [229, 50, 45, 255]);
  assert.deepEqual(px(40, 40), [0, 0, 0, 0]);
  assert.deepEqual(px(10, 10), [0, 0, 0, 0]); // the dropped mark's box is not drawn
});

test('the numbers of a second picture continue from the marks kept on the first', () => {
  const one = (text: string, at: number) => run(...gesture([at, at], [at, at]), { type: 'memo', id: 1, text }).marks;
  const first = [...one('a', 10), { ...one('b', 40)[0]!, id: 2 }];
  const second = one('c', 70);
  assert.deepEqual(
    markData(second, keptMarks(first).length).map((d) => d.n),
    [3],
  );
  const texts = marksToShapes(second, 2, false, 2).flatMap((s) => (s.kind === 'text' ? [s.text] : []));
  assert.deepEqual(texts, ['3']);
});

test('a message carries the words, then the lines of each marked picture in order, numbered across the pictures', () => {
  const one = (text: string) => run(...gesture([10, 10], [10, 10]), { type: 'memo', id: 1, text }).marks;
  const a = one('first'),
    b = [...one('second'), ...one('third').map((m) => ({ ...m, id: 2 }))];
  const text = markedText('Look  \n', [a, [], b]);
  assert.match(text, /^Look\n\n1\. first\n```marks\n/);
  assert.match(text, /\n\n2\. second\n3\. third\n```marks\n/);
  assert.deepEqual(markStarts([a, [], b]), [0, 1, 1]);
  // a picture taken out: the rest close up
  assert.match(markedText('', [b]), /^1\. second\n2\. third/);
  assert.equal(markedText('only words', []), 'only words');
});

test('a note closed with no words removes its mark, and Undo brings it back', () => {
  let s = run(...gesture([10, 10], [10, 10]), { type: 'memo', id: 1, text: 'kept' }, ...gesture([50, 50], [50, 50]));
  s = marksReducer(s, { type: 'open', id: null });
  assert.deepEqual(
    s.marks.map((m) => m.id),
    [1],
  );
  assert.equal(marksReducer(s, { type: 'undo' }).marks.length, 2);
  // a note with words stays when it closes
  assert.equal(marksReducer(marksReducer(s, { type: 'open', id: 1 }), { type: 'open', id: null }).marks.length, 1);
});

test('a seeded picture keeps its marks and numbers on; shift moves a mark and Undo puts it back', () => {
  const before = run(...gesture([10, 10], [40, 30]), { type: 'memo', id: 1, text: 'here' }).marks;
  let s = seedMarks(before);
  assert.deepEqual([s.marks, s.past.length, s.next], [before, 0, 2]);
  s = marksReducer(s, { type: 'shift', id: 1, dx: 5, dy: -3 });
  assert.deepEqual(s.marks[0]!.rect, { ...before[0]!.rect, x: before[0]!.rect.x + 5, y: before[0]!.rect.y - 3 });
  assert.deepEqual(marksReducer(s, { type: 'undo' }).marks, before);
});
