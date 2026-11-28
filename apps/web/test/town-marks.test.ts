import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import { cutTo, kindOf, markOf, papersTip, queueOf, stepsOf, tipOf, waitingOf } from '#src/town/marks.ts';
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

describe('the mark over a person', () => {
  it('is for one on hold or stopped only', () => {
    const all: Activity[] = ['working', 'waking', 'you', 'held', 'error', 'resting'];
    expect(all.map((activity) => markOf({ activity }))).toEqual([null, null, null, 'held', 'error', null]);
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

describe('a text cut short', () => {
  it('never cuts a letter of two code units in half', () => {
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
