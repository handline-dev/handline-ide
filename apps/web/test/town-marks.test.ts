import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import {
  cutTo,
  doingOf,
  doingSize,
  fitTo,
  kindOf,
  lineAt,
  linesOf,
  markOf,
  papersTip,
  queueOf,
  roomsOf,
  stepsOf,
  tipOf,
  waitingOf,
  wrapTo,
} from '#src/town/marks.ts';
import type { Person } from '#src/town/model.ts';
import { changedWork, POP_LETTERS, POP_MS, popped, popsOf, QUIET_MS, showing, type Popped } from '#src/town/pops.ts';

const person = (key: string, activity: Activity, doing: string | null = null, why: string | null = null): Person => ({
  key,
  name: key,
  activity,
  doing,
  queue: 0,
  cached: false,
  why,
});
/** Every letter as wide as the next: 2 picture pixels. */
const widthOf = (s: string) => [...s].length * 2;

describe('the mark over a person', () => {
  it('is only for one on hold, stopped or asleep', () => {
    const all: Activity[] = ['working', 'waking', 'asleep', 'you', 'held', 'error', 'resting'];
    expect(all.map(markOf)).toEqual([null, null, 'asleep', null, 'held', 'error', null]);
  });
});

describe('the line under a name', () => {
  it('says what one at a desk is doing, and nothing for the others', () => {
    expect(doingOf(person('a', 'working', 'Plan the notes'))).toBe('Plan the notes');
    expect(doingOf(person('a', 'you', ' Ask about the name '))).toBe('Ask about the name');
    expect(doingOf(person('a', 'working'))).toBeNull();
    expect(doingOf(person('a', 'you'))).toBeNull();
    for (const idle of ['resting', 'asleep', 'waking'] as const) expect(doingOf(person('a', idle, 'Old work'))).toBeNull();
  });

  it('says why one on hold or stopped waits when it has no work to name, else the state word', () => {
    expect(doingOf(person('a', 'held', 'Write the guide', 'Usage limit'))).toBe('Write the guide');
    expect(doingOf(person('a', 'held', null, 'Usage limit'))).toBe('Usage limit');
    expect(doingOf(person('a', 'held'))).toBe('On hold');
    expect(doingOf(person('a', 'error', '  '))).toBe('Stopped');
  });

  it('is 9px on screen, shrinks with the room and goes once it would be under 8px', () => {
    expect(doingSize(3)).toBeCloseTo(3);
    expect(doingSize(2)! * 2).toBeCloseTo(9);
    // Capped in picture pixels as the names are, so it grows no further in a small room.
    expect(doingSize(1.6)).toBeCloseTo(5.3);
    expect(doingSize(1.5)).toBeNull();
    expect(doingSize(0.5)).toBeNull();
  });
});

describe('what hovering a person says', () => {
  it('names its work, or its state in a word', () => {
    expect(tipOf(person('ui', 'working', 'Edit the sheet'))).toBe('ui · Edit the sheet');
    expect(tipOf(person('ui', 'asleep'))).toBe('ui · Asleep');
  });

  it('says the state and why of one that needs a look, each once', () => {
    expect(tipOf(person('ui', 'held', 'Edit the sheet', 'Usage limit'))).toBe('ui · On hold · Edit the sheet · Usage limit');
    expect(tipOf(person('ui', 'error'))).toBe('ui · Stopped');
    expect(tipOf(person('ui', 'error', null, 'Stopped'))).toBe('ui · Stopped');
    expect(tipOf(person('ui', 'you', 'Pick a name'))).toBe('ui · Waiting for you · Pick a name');
  });
});

describe('the room a line has', () => {
  const o = { gap: 4, row: 5, most: 100 };

  it('stops short of the nearest neighbour on its row, whichever side', () => {
    const at = [
      { x: 100, y: 50 },
      { x: 133, y: 50 },
      { x: 180, y: 52 },
    ];
    expect(roomsOf(at, 400, o)).toEqual([29, 29, 43]);
  });

  it('is not held back by another row, and stays inside the picture and under the most', () => {
    const at = [
      { x: 100, y: 50 },
      { x: 110, y: 90 },
      { x: 20, y: 130 },
      { x: 395, y: 170 },
    ];
    expect(roomsOf(at, 400, o)).toEqual([100, 100, 36, 6]);
    expect(roomsOf([{ x: 1, y: 0 }], 400, o)).toEqual([0]);
  });
});

describe('a text cut to fit', () => {
  it('stays whole when it fits', () => {
    expect(fitTo('Plan', 8, widthOf)).toBe('Plan');
  });

  it('keeps its longest start that fits with the ellipsis, never over the room', () => {
    for (let room = 4; room < 40; room++) {
      const cut = fitTo('Plan the release notes', room, widthOf)!;
      expect(widthOf(cut)).toBeLessThanOrEqual(room);
      expect(cut.endsWith('…')).toBe(true);
    }
    expect(fitTo('Plan the release notes', 20, widthOf)).toBe('Plan the…');
    expect(fitTo('Plan the release notes', 22, widthOf)).toBe('Plan the r…');
  });

  it('is nothing when not even a letter fits', () => {
    expect(fitTo('Plan the release notes', 3, widthOf)).toBeNull();
    expect(fitTo('Plan the release notes', 0, widthOf)).toBeNull();
  });

  it('never cuts a letter of two code units in half', () => {
    expect(fitTo('😀😀😀😀', 6, widthOf)).toBe('😀😀…');
    expect(cutTo('😀😀😀😀', 3)).toBe('😀😀…');
  });

  it('counts letters for a bubble, the ellipsis among them', () => {
    expect(cutTo('  Run the tests  ', 40)).toBe('Run the tests');
    const long = cutTo('Read the import logs of the last three nightly runs', POP_LETTERS);
    // The space before the cut goes, so the ellipsis follows a letter.
    expect(long).toBe('Read the import logs of the last three…');
    expect([...cutTo('abcdefghij', 5)]).toHaveLength(5);
  });
});

describe('the lines of an office', () => {
  const names = [
    { key: 'a', x: 100, y: 50 },
    { key: 'b', x: 133, y: 50 },
    { key: 'c', x: 166, y: 50 },
    { key: 'd', x: 100, y: 110 },
  ];
  const look = { w: 400, size: 4, below: 5, heads: [], widthOf };
  const people = [
    person('a', 'working', 'Plan the release notes'),
    person('b', 'resting', 'Old work'),
    person('c', 'held'),
    person('d', 'working', 'Read the logs'),
  ];

  it('wraps each within the room its row leaves and skips whoever has nothing to say', () => {
    expect(linesOf(names, [], people, look)).toEqual([
      // 33 apart less a gap of 3.6: 29.4 wide, 14 letters of 2 a line.
      { key: 'a', x: 100, y: 50, lines: ['Plan the', 'release notes'] },
      { key: 'c', x: 166, y: 50, lines: ['On hold'] },
      { key: 'd', x: 100, y: 110, lines: ['Read the logs'] },
    ]);
  });

  it('keeps one line where a second would reach the mark over a head of the row below', () => {
    // Two lines under a name 5 tall at 50 end at 64.6; a mark reaches 7 over its head's top.
    const under = (heads: { x: number; y: number }[]) => linesOf(names, [], people, { ...look, heads })[0].lines;
    expect(under([{ x: 100, y: 70 }])).toEqual(['Plan the rele…']);
    expect(under([{ x: 100, y: 80 }])).toEqual(['Plan the', 'release notes']);
    // Neither a head far to the side nor the one over the name itself is in the way.
    expect(
      under([
        { x: 200, y: 70 },
        { x: 100, y: 20 },
      ]),
    ).toEqual(['Plan the', 'release notes']);
  });

  it('reads the newest people, by key, and leaves room for a name that gets no line', () => {
    const now = [person('d', 'working', 'Read the import logs of the night')];
    // The founder's desk stands on the last row, 30 to the right.
    expect(linesOf(names, [{ x: 130, y: 112 }], now, look)).toEqual([{ key: 'd', x: 100, y: 110, lines: ['Read the', 'import logs…'] }]);
    // With nobody beside it a line still ends within thirteen times its height: 52 wide here.
    expect(linesOf(names, [], now, look)).toEqual([{ key: 'd', x: 100, y: 110, lines: ['Read the import logs of', 'the night'] }]);
    expect(linesOf(names, [], [], look)).toEqual([]);
  });

  it('stands the lines one under the other, below the name', () => {
    expect(lineAt(50, 5, 4, 0)).toBeCloseTo(59.2);
    expect(lineAt(50, 5, 4, 1)).toBeCloseTo(63.6);
  });
});

describe('a text on two lines', () => {
  it('stays on one when it fits', () => {
    expect(wrapTo('Plan the notes', 28, widthOf)).toEqual(['Plan the notes']);
  });

  it('breaks between words, and ends the second line in an ellipsis when it is too long too', () => {
    expect(wrapTo('Plan the release notes', 30, widthOf)).toEqual(['Plan the', 'release notes']);
    const long = wrapTo('Read the import logs of the last three nightly runs', 30, widthOf);
    expect(long).toEqual(['Read the import', 'logs of the la…']);
    for (const line of long) expect(widthOf(line)).toBeLessThanOrEqual(30);
  });

  it('cuts a first word too long for a line where the line ends', () => {
    expect(wrapTo('Supercalifragilistic word', 20, widthOf)).toEqual(['Supercalif', 'ragilisti…']);
  });

  it('is nothing when not even a letter fits', () => {
    expect(wrapTo('Plan the release notes', 1, widthOf)).toEqual([]);
  });
});

describe("a person's card", () => {
  it('counts the tasks waiting behind the one it holds', () => {
    expect(waitingOf({ ...person('a', 'working'), queue: 7, task: 12 })).toBe('6 tasks waiting');
    expect(waitingOf({ ...person('a', 'working'), queue: 2, task: 12 })).toBe('1 task waiting');
    expect(waitingOf({ ...person('a', 'resting'), queue: 3, task: null })).toBe('3 tasks waiting');
    expect(waitingOf({ ...person('a', 'working'), queue: 1, task: 12 })).toBeNull();
    expect(waitingOf(person('a', 'asleep'))).toBeNull();
  });

  it('says what it runs on and how full its context is, as far as known', () => {
    expect(kindOf({ ...person('a', 'working'), model: 'sonnet', effort: 'high', ctx: 24.6 })).toBe('sonnet · high · Context 25%');
    expect(kindOf({ ...person('a', 'working'), model: 'opus', effort: null, ctx: null })).toBe('opus');
    expect(kindOf({ ...person('a', 'working'), ctx: 0 })).toBe('Context 0%');
    expect(kindOf(person('a', 'working'))).toBeNull();
  });
});

describe('the steps of a task', () => {
  it('are the count and the part done', () => {
    expect(stepsOf({ done: 3, total: 7 })).toEqual({ text: '3/7', part: 3 / 7 });
    expect(stepsOf({ done: 0, total: 4 })).toEqual({ text: '0/4', part: 0 });
  });

  it('are none without a count, and never over the whole', () => {
    expect(stepsOf(undefined)).toBeNull();
    expect(stepsOf(null)).toBeNull();
    expect(stepsOf({ done: 2, total: 0 })).toBeNull();
    expect(stepsOf({ done: 9, total: 4 })).toEqual({ text: '4/4', part: 1 });
    expect(stepsOf({ done: -1, total: 4 })).toEqual({ text: '0/4', part: 0 });
  });
});

describe('the papers on a desk', () => {
  const task = (id: number, to: string, state: string, title = `Task ${id}`) => ({ id, to, state, title });

  it('are the open tasks their agent owns: the one being done first, then the oldest', () => {
    const tasks = [task(9, 'a', 'queued'), task(7, 'a', 'doing'), task(3, 'a', 'queued'), task(5, 'b', 'doing'), task(2, 'a', 'draft')];
    expect(queueOf(tasks, 'a')).toEqual(['Task 7', 'Task 3', 'Task 9']);
    expect(queueOf(tasks, 'c')).toEqual([]);
  });

  it('name up to five titles on hover and count the rest', () => {
    const titles = ['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven'];
    expect(papersTip('lead', titles.slice(0, 2))).toBe("lead's tasks\n· One\n· Two");
    expect(papersTip('lead', titles.slice(0, 5))).toBe("lead's tasks\n· One\n· Two\n· Three\n· Four\n· Five");
    expect(papersTip('lead', titles)).toBe("lead's tasks\n· One\n· Two\n· Three\n· Four\n· Five\n+2 more");
    // Not read yet, or none: as it was before the titles were known.
    expect(papersTip('lead', [])).toBe("lead's tasks");
  });
});

describe('work that changes', () => {
  it('is news only for one there both times whose work is now something else', () => {
    const was = [person('a', 'working', 'One'), person('b', 'working', 'Same'), person('c', 'working', 'Old'), person('d', 'resting')];
    const now = [person('a', 'working', 'Two'), person('b', 'working', 'Same'), person('c', 'resting'), person('d', 'working', 'New')];
    expect(changedWork(was, [...now, person('e', 'working', 'Came in')])).toEqual([
      { key: 'a', text: 'Two' },
      { key: 'd', text: 'New' },
    ]);
    expect(changedWork(was, was)).toEqual([]);
  });

  const first: Popped = { people: null, pops: [] };

  it('pops nothing when the office is first shown', () => {
    const seen = popped(first, [person('a', 'working', 'One')], 1000);
    expect(seen.pops).toEqual([]);
    expect(seen.people).toHaveLength(1);
  });

  it('pops the new work, cut short, and shows it for four seconds', () => {
    const seen = popped(first, [person('a', 'working', 'One')], 1000);
    const long = 'Read the import logs of the last three nightly runs';
    const next = popped(seen, [person('a', 'working', long)], 2000);
    expect(next.pops).toEqual([{ key: 'a', text: 'Read the import logs of the last three…', at: 2000 }]);
    expect(showing(next.pops, 2000 + POP_MS - 1)).toHaveLength(1);
    expect(showing(next.pops, 2000 + POP_MS)).toEqual([]);
  });

  it('keeps one whose work changes all the time quiet between bubbles, and forgets old ones', () => {
    let seen = popped(first, [person('a', 'working', 'One'), person('b', 'working', 'B1')], 0);
    seen = popped(seen, [person('a', 'working', 'Two'), person('b', 'working', 'B1')], 1000);
    // a changes again five seconds on: still quiet. b's first change pops.
    seen = popped(seen, [person('a', 'working', 'Three'), person('b', 'working', 'B2')], 6000);
    expect(seen.pops.map((p) => [p.key, p.text, p.at])).toEqual([
      ['a', 'Two', 1000],
      ['b', 'B2', 6000],
    ]);
    // Once the quiet is over a's next change pops, and its old bubble is gone.
    seen = popped(seen, [person('a', 'working', 'Four'), person('b', 'working', 'B2')], 1000 + QUIET_MS);
    expect(seen.pops.map((p) => [p.key, p.text])).toEqual([
      ['b', 'B2'],
      ['a', 'Four'],
    ]);
  });

  it('keeps the same list when a look changes nothing', () => {
    const seen = popped(popped(first, [person('a', 'working', 'One')], 0), [person('a', 'working', 'Two')], 10);
    expect(popped(seen, [person('a', 'working', 'Two')], 20).pops).toBe(seen.pops);
  });
});

describe("an office's bubbles as time passes", () => {
  beforeEach(() => void vi.useFakeTimers());
  afterEach(() => void vi.useRealTimers());

  it('shows a bubble on a change, tells whoever follows, and takes it away after four seconds', () => {
    let at = 0;
    const pops = popsOf(() => at);
    const told = vi.fn();
    pops.subscribe(told);
    pops.see([person('a', 'working', 'One')]);
    expect(pops.get()).toEqual([]);
    expect(told).not.toHaveBeenCalled();

    at = 500;
    pops.see([person('a', 'working', 'Two')]);
    expect(pops.get()).toEqual([{ key: 'a', text: 'Two', at: 500 }]);
    expect(told).toHaveBeenCalledTimes(1);
    // A look that changes nothing keeps the same list: nothing is drawn again.
    const shown = pops.get();
    pops.see([person('a', 'working', 'Two')]);
    expect(pops.get()).toBe(shown);
    expect(told).toHaveBeenCalledTimes(1);

    at = 500 + POP_MS;
    vi.advanceTimersByTime(POP_MS);
    expect(pops.get()).toEqual([]);
    expect(told).toHaveBeenCalledTimes(2);
  });

  it('forgets everything when the office goes away, so coming back pops nothing', () => {
    let at = 0;
    const pops = popsOf(() => at);
    pops.see([person('a', 'working', 'One')]);
    at = 100;
    pops.see([person('a', 'working', 'Two')]);
    expect(pops.get()).toHaveLength(1);
    pops.see(null);
    expect(pops.get()).toEqual([]);
    expect(vi.getTimerCount()).toBe(0);
    pops.see([person('a', 'working', 'Three')]);
    expect(pops.get()).toEqual([]);
  });
});
