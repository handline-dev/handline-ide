import { describe, expect, it } from 'vitest';
import { badgeScale, cut, platesOf, signFont, type Plate, type Post } from '#src/town/signs.ts';
import { eased, inPane, keepPoint, levelOf, rangeOf, reachOf, scaleAt, settle, stepped, zoomed, zooms, type Pair } from '#src/town/zoom.ts';

// A town three times as wide as its pane, and two and a half times as tall.
const size: Pair = [3000, 1500];
const pane: Pair = [1000, 600];
const range = rangeOf(size, pane);

describe('the scales a pane allows', () => {
  it('go from the whole town in the pane to the picture at its own pixels', () => {
    expect(range.min).toBeCloseTo(1 / 3);
    expect(range.max).toBe(1);
    expect(zooms(range)).toBe(true);
    expect(size[0] * range.min).toBeLessThanOrEqual(pane[0] + 1e-9);
    expect(size[1] * range.min).toBeLessThanOrEqual(pane[1] + 1e-9);
  });

  it('leave nothing to zoom when the whole town fits at its own pixels or larger', () => {
    const small = rangeOf([400, 200], pane);
    expect(small).toEqual({ min: 2.5, max: 2.5 });
    expect(zooms(small)).toBe(false);
    expect(scaleAt(0, small)).toBe(2.5);
    expect(levelOf(1, small)).toBe(1);
  });

  it('are the close-up alone for a pane not measured yet', () => {
    expect(rangeOf(size, [0, 0])).toEqual({ min: 1, max: 1 });
  });
});

describe('the zoom level', () => {
  it('is the whole town at 0 and the close-up at 1, a scale and its level agreeing between', () => {
    expect(scaleAt(0, range)).toBeCloseTo(range.min);
    expect(scaleAt(1, range)).toBe(1);
    expect(levelOf(scaleAt(0.4, range), range)).toBeCloseTo(0.4);
    expect(scaleAt(7, range)).toBe(1);
    // The ends are exact whatever the pane, so the close-up is drawn as the close-up.
    for (const narrow of [rangeOf([2963, 1617], [420, 700]), rangeOf([2963, 1617], [1280, 757]), rangeOf([1777, 2301], [900, 771])]) {
      expect(scaleAt(1, narrow)).toBe(1);
      expect(scaleAt(0, narrow)).toBe(narrow.min);
    }
    expect(levelOf(50, range)).toBe(1);
    expect(levelOf(0.001, range)).toBe(0);
  });

  it('zooms by a factor, never past either end, and rests on an end when nearly there', () => {
    expect(scaleAt(zoomed(1, 0.8, range), range)).toBeCloseTo(0.8);
    expect(zoomed(1, 1.5, range)).toBe(1);
    expect(zoomed(0, 0.5, range)).toBe(0);
    expect(zoomed(1, 0.001, range)).toBe(0);
    expect(zoomed(0.99, 1.005, range)).toBe(1);
    expect(zoomed(0.01, 0.999, range)).toBe(0);
  });

  it('leaves an end by steps too small to pass the rest alone, as a pinch or a slow wheel makes them', () => {
    let out = 1;
    let near = 0;
    for (let i = 0; i < 60; i++) {
      out = zoomed(out, 0.99, range);
      near = zoomed(near, 1.01, range);
    }
    expect(scaleAt(out, range)).toBeCloseTo(0.99 ** 60);
    expect(scaleAt(near, range)).toBeCloseTo(range.min * 1.01 ** 60);
  });

  it('steps by the buttons through even stops, from where the wheel left it', () => {
    expect(stepped(1, -1)).toBeCloseTo(2 / 3);
    expect(stepped(2 / 3, -1)).toBeCloseTo(1 / 3);
    expect(stepped(1 / 3, -1)).toBe(0);
    expect(stepped(0, -1)).toBe(0);
    expect(stepped(0, 1)).toBeCloseTo(1 / 3);
    expect(stepped(1, 1)).toBe(1);
    expect(stepped(0.5, 1)).toBeCloseTo(2 / 3);
    expect(stepped(0.5, -1)).toBeCloseTo(1 / 3);
  });
});

describe('where the picture sits', () => {
  it('keeps the point under the pointer still through a zoom', () => {
    const o: Pair = [-800, -300];
    const p: Pair = [250, 420];
    const [from, to] = [1, 0.6];
    const under = (at: Pair, scale: number) => [(p[0] - at[0]) / scale, (p[1] - at[1]) / scale];
    const next = keepPoint(o, p, from, to);
    expect(under(next, to)[0]).toBeCloseTo(under(o, from)[0]);
    expect(under(next, to)[1]).toBeCloseTo(under(o, from)[1]);
  });

  it('never shows past the picture edges, and centres a picture smaller than the pane', () => {
    expect(inPane([50, 40], size, pane, 1)).toEqual([0, 0]);
    expect(inPane([-9000, -9000], size, pane, 1)).toEqual([-2000, -900]);
    expect(inPane([-500, -100], size, pane, 1)).toEqual([-500, -100]);
    const [x, y] = inPane([-500, -100], size, pane, range.min);
    expect(x).toBeCloseTo(0);
    expect(y).toBeCloseTo(50); // 1500 / 3 = 500 tall in a pane 600 tall
  });

  it('stays on the founder in the close-up, as far as the edges allow', () => {
    expect(reachOf(1, 1000)).toBe(0);
    expect(settle([0, 0], [1500, 750], 1, size, pane)).toEqual([-1000, -450]);
    // Near the town's corner the picture stops at its edge and the founder walks off the middle.
    expect(settle([-1000, -450], [100, 80], 1, size, pane)).toEqual([0, 0]);
    expect(settle([0, 0], [2950, 1480], 1, size, pane)).toEqual([-2000, -900]);
  });

  it('zoomed out, moves only when the founder would leave its reach of the middle', () => {
    const level = 0.5;
    const scale = scaleAt(level, range);
    const reach = reachOf(level, pane[0]);
    expect(reach).toBeGreaterThan(0);
    expect(reach).toBeLessThan(pane[0] / 2);
    const o: Pair = [-300, -100];
    // In reach: the picture stays where the zoom left it.
    const inside: Pair = [(pane[0] / 2 + reach - 10 - o[0]) / scale, (pane[1] / 2 - o[1]) / scale];
    expect(settle(o, inside, level, size, pane)).toEqual(o);
    // Past it: the picture moves just enough to bring the founder back to the edge of its reach.
    const outside: Pair = [(pane[0] / 2 + reach + 40 - o[0]) / scale, (pane[1] / 2 - o[1]) / scale];
    const moved = settle(o, outside, level, size, pane);
    expect(moved[0]).toBeCloseTo(o[0] - 40);
    expect(moved[1]).toBeCloseTo(o[1]);
  });

  it('shows the whole town whatever the founder does, and is unmoved with no founder', () => {
    const whole = settle([-700, -200], [2900, 1400], 0, size, pane);
    expect(whole[0]).toBeCloseTo(0);
    expect(whole[1]).toBeCloseTo(50);
    expect(settle([-700, -200], null, 1, size, pane)).toEqual([-700, -200]);
  });
});

// Three buildings in a row, 100 picture pixels wide each, their drawn signs 40 wide over their doors.
const post = (i: number, name: string, sub: string, marks: string[] = []): Post => ({
  key: `k${i}`,
  box: { x: i * 100 + 30, y: 200, w: 40, h: 9 },
  room: 100,
  row: 0,
  name,
  sub,
  marks,
});
const wide = (p: Plate) => [p.x, p.x + p.w];
const apart = (plates: Plate[]) => plates.every((p, i) => i === 0 || wide(plates[i - 1])[1] <= wide(p)[0]);

describe('the signs at a scale', () => {
  it('keep their text about as tall on screen however far out, and their badges no smaller than most of their size', () => {
    expect(signFont(1)).toBe(8.5);
    expect(signFont(2)).toBe(8.5);
    for (const s of [0.9, 0.6, 0.4, 0.2]) {
      expect(signFont(s) * s).toBeGreaterThanOrEqual(8.5);
      expect(signFont(s) * s).toBeLessThanOrEqual(10.5 + 1e-9);
      expect(badgeScale(s) * s).toBeGreaterThanOrEqual(0.85 - 1e-9);
    }
    expect(badgeScale(1)).toBe(1);
    expect(badgeScale(1.4)).toBe(1);
  });

  it('are the buildings own signs in the close-up: the name alone, badges from the right end', () => {
    const [p] = platesOf([post(0, 'alpha', '2 of 3 at work', ['4'])], 1);
    expect(p).toMatchObject({ drawn: true, name: 'alpha', sub: null, x: 30, y: 200, w: 40, h: 9, font: 8.5 });
    expect(p.marks).toEqual({ x: 73, y: 204.5, scale: 1 });
  });

  it('show the line under the name where there is room for it', () => {
    const plates = platesOf([post(0, 'alpha', 'Lights off'), post(2, 'gamma', 'Lights off')], 0.8);
    expect(plates.map((p) => p.sub)).toEqual(['Lights off', 'Lights off']);
    expect(plates.every((p) => !p.drawn && p.h > p.font * 2)).toBe(true);
    expect(apart(plates)).toBe(true);
  });

  it('drop the line under the name where two signs would meet, and keep the badges', () => {
    const posts = [post(0, 'alpha', '12 of 14 at work in all', ['4']), post(1, 'beta', '12 of 14 at work in all'), post(2, 'gamma', '')];
    const plates = platesOf(posts, 0.8);
    expect(plates.map((p) => p.sub)).toEqual([null, null, null]);
    expect(plates.map((p) => p.name)).toEqual(['alpha', 'beta', 'gamma']);
    expect(apart(plates)).toBe(true);
    // The first sign's badge sits between its plate and the next sign.
    expect(plates[0].marks.x).toBeGreaterThanOrEqual(wide(plates[0])[1]);
    expect(plates[0].marks.x).toBeLessThan(wide(plates[1])[0]);
  });

  it('keep to their own buildings where even the names would meet: the name cut, the badges over it', () => {
    const posts = [
      post(0, 'a-very-long-team-name', 'Lights off', ['12', '!']),
      post(1, 'another-long-team-name', 'Lights off'),
      post(2, 'c', ''),
    ];
    const plates = platesOf(posts, 0.4);
    expect(plates[0].name.endsWith('…')).toBe(true);
    expect(plates[1].name.endsWith('…')).toBe(true);
    expect(plates[2].name).toBe('c');
    expect(apart(plates)).toBe(true);
    plates.slice(0, 2).forEach((p, i) => {
      expect(p.x).toBeGreaterThanOrEqual(i * 100);
      expect(p.x + p.w).toBeLessThanOrEqual((i + 1) * 100);
    });
    // The badges are a row over the plate, inside the building's width.
    const { marks, y } = plates[0];
    expect(marks.y + 8 * marks.scale).toBeLessThanOrEqual(y);
    expect(marks.x).toBeGreaterThanOrEqual(0);
    expect(marks.x + marks.scale * (21 + 2 + 16)).toBeLessThanOrEqual(100);
  });

  it('keep the badges beside a name that fits its own building with them', () => {
    const posts = [post(0, 'ab', 'Lights off', ['4']), post(1, 'another-long-team-name', 'Lights off')];
    const [first, second] = platesOf(posts, 0.6);
    expect(first.name).toBe('ab');
    expect(first.marks.y).toBe(first.line);
    expect(first.marks.x).toBeGreaterThanOrEqual(first.x + first.w);
    expect(first.marks.x + first.marks.scale * 16).toBeLessThanOrEqual(100);
    expect(second.x).toBeGreaterThanOrEqual(100);
  });

  it('cuts a name to the room it has, never to nothing', () => {
    const letter = (text: string) => text.length; // every letter as wide as it is tall
    expect(cut('handline', 80, 10, letter)).toBe('handline');
    expect(cut('handline', 50, 10, letter)).toBe('hand…');
    expect(cut('hand line', 60, 10, letter)).toBe('hand…');
    expect(cut('handline', 1, 10, letter)).toBe('h…');
  });

  it('read their short word where the name would have to be cut, and their name where it fits', () => {
    const posts = [{ ...post(0, '+ New team to add', '', []), short: '+' }, post(1, 'another-long-team-name', '')];
    expect(platesOf(posts, 0.4)[0].name).toBe('+');
    // With nothing beside it to meet, it keeps its name.
    expect(platesOf([posts[0]], 0.4)[0].name).toBe('+ New team to add');
    expect(platesOf([{ ...post(0, 'ab', ''), short: '+' }, posts[1]], 0.4)[0].name).toBe('ab');
  });

  it('show the line under the names in a whole row or in none of it', () => {
    // Two of the row would meet with their lines, so the third drops its line too; the row behind has room and keeps its lines.
    const front = [post(0, 'alpha', '12 of 14 at work in all'), post(1, 'beta', '12 of 14 at work in all'), post(3, 'delta', 'Lights off')];
    const behind = [post(0, 'one', 'Lights off'), post(2, 'two', 'Lights off')].map((p) => ({ ...p, key: `b${p.key}`, row: 1 }));
    const plates = platesOf([...front, ...behind], 0.8);
    expect(plates.map((p) => p.sub)).toEqual([null, null, null, 'Lights off', 'Lights off']);
  });

  it('rest on the roof edge zoomed out, clear of the door and the windows, and on the drawn sign where there is no roof', () => {
    const [roofed, bare] = platesOf([{ ...post(0, 'alpha', 'Lights off'), roof: 120 }, post(2, 'gamma', 'Lights off')], 0.5);
    // Its bottom just over the top of the front, however many lines it has.
    expect(roofed.sub).toBe('Lights off');
    expect(roofed.y + roofed.h).toBeLessThanOrEqual(120);
    expect(roofed.y + roofed.h).toBeGreaterThan(120 - 8);
    expect(roofed.marks.y).toBe(roofed.line);
    const [alone] = platesOf([{ ...post(0, 'alpha', ''), roof: 120 }], 0.5);
    expect(alone.y + alone.h).toBeCloseTo(roofed.y + roofed.h);
    // No roof: over the middle of the drawn sign, as before.
    expect(bare.line).toBeCloseTo(204.5);
    // In the close-up every name is on its drawn sign, roof or not.
    expect(platesOf([{ ...post(0, 'alpha', 'Lights off'), roof: 120 }], 1)[0]).toMatchObject({ drawn: true, y: 200, h: 9 });
  });
});

describe('a step by a button', () => {
  it('eases from one level to the other: there at the end, most of the way early, never past either', () => {
    expect(eased(1, 0, 0)).toBe(1);
    expect(eased(1, 0, 1)).toBe(0);
    expect(eased(0, 1, 0.5)).toBeCloseTo(0.875);
    expect(eased(0.2, 0.8, 2)).toBeCloseTo(0.8);
    expect(eased(0.2, 0.8, -1)).toBeCloseTo(0.2);
    const steps = [0, 0.25, 0.5, 0.75, 1].map((k) => eased(1, 1 / 3, k));
    expect(steps.every((v, i) => i === 0 || v < steps[i - 1])).toBe(true);
  });
});
