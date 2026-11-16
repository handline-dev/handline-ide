import { test } from 'node:test';
import assert from 'node:assert/strict';
import { shapesToSvg, textSize, type Point, type Shape } from '#src/annotation.ts';
import { exportAnnotated, type Bitmap } from '#src/annotation-raster.ts';

const RED = '#ff0000';
const blank = (width: number, height: number): Bitmap => ({ width, height, data: new Uint8ClampedArray(width * height * 4) });
const px = (b: Bitmap, x: number, y: number) => Array.from(b.data.slice((y * b.width + x) * 4, (y * b.width + x) * 4 + 4));
const p = (x: number, y: number): Point => ({ x, y });
let id = 0;
const base = (color = '#e5322d', width = 6) => ({ id: id++, color, width });
const two = (kind: 'arrow' | 'box' | 'ellipse', from: Point, to: Point, color?: string, width?: number): Shape => ({
  ...base(color, width),
  kind,
  from,
  to,
});
const text = (at: Point, words: string): Shape => ({ ...base(), kind: 'text', at, text: words, size: textSize(6) });
const note = (from: Point, to: Point, words: string, color?: string, width?: number): Shape => ({
  ...base(color, width),
  kind: 'note',
  from,
  to,
  text: words,
});

test('export paints the shapes and leaves the source alone', () => {
  const src = blank(20, 20);
  const out = exportAnnotated(src, [two('box', p(2, 2), p(15, 15), RED, 4)]);
  assert.deepEqual(px(out, 2, 8), [255, 0, 0, 255]);
  assert.deepEqual(px(out, 8, 8), [0, 0, 0, 0]);
  assert.deepEqual(px(src, 2, 8), [0, 0, 0, 0]);
});

test('arrow and text draw something inside the bitmap', () => {
  const out = exportAnnotated(blank(50, 40), [two('arrow', p(2, 2), p(40, 2)), text(p(2, 10), 'Hi')]);
  assert.ok(out.data.some((v, i) => i % 4 === 3 && v === 255));
  assert.deepEqual(px(out, 20, 2), [229, 50, 45, 255]);
});

test('scale shrinks positions and widths with the bitmap', () => {
  const out = exportAnnotated(blank(100, 100), [two('box', p(20, 20), p(150, 150), RED, 8)], 0.5);
  assert.deepEqual(px(out, 10, 50), [255, 0, 0, 255]);
  assert.deepEqual(px(out, 50, 50), [0, 0, 0, 0]);
  assert.deepEqual(px(out, 10, 90), [0, 0, 0, 0]);
});

test('svg data has a path per shape and keeps the model units', () => {
  const shapes: Shape[] = [
    { ...base(), kind: 'pen', points: [p(1, 2), p(3, 4)] },
    two('box', p(0, 0), p(10, 5)),
    two('arrow', p(0, 0), p(100, 0)),
    text(p(5, 6), 'hi'),
  ];
  const svg = shapesToSvg(shapes, 640, 480);
  assert.equal(svg.viewBox, '0 0 640 480');
  assert.equal(svg.elements.length, 4);
  assert.deepEqual(svg.elements[0], { type: 'path', d: 'M1 2L3 4', stroke: '#e5322d', strokeWidth: 6 });
  assert.deepEqual(svg.elements[1]?.type === 'path' && svg.elements[1].d, 'M0 0L10 0L10 5L0 5Z');
  assert.equal(svg.elements[2]?.type === 'path' && svg.elements[2].d.startsWith('M0 0L100 0M100 0L'), true);
  assert.deepEqual(svg.elements[3], { type: 'text', x: 5, y: 6, fontSize: 34.29, fill: '#e5322d', text: 'HI' });
});

test('a stroke of a billion pixels is drawn only where it is visible, and fast', () => {
  const line = (y: number): Shape => ({ ...base(RED), kind: 'pen', points: [p(-1e9, y), p(1e9, y)] });
  const t = Date.now();
  const out = exportAnnotated(blank(30, 10), [line(5)]);
  assert.ok(Date.now() - t < 1000);
  assert.deepEqual(px(out, 15, 5), [255, 0, 0, 255]);
  assert.ok(exportAnnotated(blank(30, 10), [line(500)]).data.every((v) => v === 0));
});

test('a circle is drawn as an outline inside its box: the rim is painted, the middle and the corner are not', () => {
  const circle = two('ellipse', p(10, 10), p(90, 90), RED, 4);
  const out = exportAnnotated(blank(100, 100), [circle]);
  assert.deepEqual(px(out, 10, 50), [255, 0, 0, 255]);
  assert.deepEqual(px(out, 50, 10), [255, 0, 0, 255]);
  assert.deepEqual(px(out, 50, 50), [0, 0, 0, 0]);
  assert.deepEqual(px(out, 12, 12), [0, 0, 0, 0]);
  const first = shapesToSvg([circle], 100, 100).elements[0];
  assert.equal(first?.type === 'path' && first.d.endsWith('Z'), true);
});

test('a note draws its number in the preview, counted among the notes only', () => {
  const shapes = [text(p(0, 0), 'hi'), note(p(10, 10), p(10, 10), 'a'), note(p(50, 50), p(90, 80), 'b')];
  const numbers = shapesToSvg(shapes, 100, 100).elements.flatMap((e) => (e.type === 'text' && e.text !== 'HI' ? [e.text] : []));
  assert.deepEqual(numbers, ['1', '2']);
});

test('export outlines a region note and leaves its inside alone', () => {
  const out = exportAnnotated(blank(80, 80), [note(p(10, 10), p(60, 60), 'here', RED, 2)]);
  assert.deepEqual(px(out, 10, 30), [255, 0, 0, 255]);
  assert.deepEqual(px(out, 40, 40), [0, 0, 0, 0]);
});
