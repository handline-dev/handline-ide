import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CHEER_MS, cheersOf, finishedWork } from '#src/town/cheers.ts';
import { LOOKS, lookNow, pickLook } from '#src/town/look.ts';

describe('a finished task', () => {
  const now = Date.parse('2030-05-06T15:00:00Z');
  const ago = (s: number) => new Date(now - s * 1000).toISOString();
  const task = (state: string, done: string | null, to = 'web') => ({ to, state, done });

  it('is cheered for when it was just done by an agent', () => {
    expect(finishedWork(task('done', ago(2)), now, now - 10_000)).toBe(true);
  });
  it('is not when it is not done, has nobody, or was done a while ago or before the screen opened', () => {
    expect(finishedWork(task('doing', ago(2)), now)).toBe(false);
    expect(finishedWork(task('done', null), now)).toBe(false);
    expect(finishedWork(task('done', ago(2), ''), now)).toBe(false);
    expect(finishedWork(task('done', ago(61)), now)).toBe(false);
    expect(finishedWork(task('done', ago(30)), now, now - 10_000)).toBe(false);
    // The daemon's clock a little behind this screen's.
    expect(finishedWork(task('done', ago(13)), now, now - 10_000)).toBe(true);
  });
});

describe('the cheers of an office', () => {
  beforeEach(() => void vi.useFakeTimers());
  afterEach(() => void vi.useRealTimers());

  it('cheers for CHEER_MS, and a second finish starts the cheer over', () => {
    let at = 1000;
    const cheers = cheersOf(() => at);
    const heard = vi.fn();
    cheers.subscribe(heard);
    cheers.cheer('web');
    expect(cheers.get().get('web')).toBe(1000);
    at += CHEER_MS - 500;
    vi.advanceTimersByTime(CHEER_MS - 500);
    cheers.cheer('web');
    expect(cheers.get().get('web')).toBe(at);
    vi.advanceTimersByTime(CHEER_MS - 1);
    expect(cheers.get().has('web')).toBe(true);
    vi.advanceTimersByTime(1);
    expect(cheers.get().size).toBe(0);
    expect(heard).toHaveBeenCalledTimes(3);
  });
  it('stops every cheer when the office goes', () => {
    const cheers = cheersOf();
    cheers.cheer('web');
    cheers.cheer('api');
    cheers.stop();
    expect(cheers.get().size).toBe(0);
    vi.advanceTimersByTime(CHEER_MS);
    expect(cheers.get().size).toBe(0);
  });
});

describe("the founder's look", () => {
  afterEach(() => pickLook(0));

  it('is its own figure first, then each of the walking people', () => {
    expect(LOOKS[0].sheet.url).toMatch(/founder\.png$/);
    expect(LOOKS.slice(1).map((l) => l.row)).toEqual([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]);
  });
  it('is kept on this device, and a pick that is no look is its own', () => {
    pickLook(3);
    expect(lookNow()).toBe(3);
    expect(localStorage.getItem('web:town.look')).toBe('3');
    pickLook(LOOKS.length);
    expect(lookNow()).toBe(0);
    expect(localStorage.getItem('web:town.look')).toBeNull();
  });
});
