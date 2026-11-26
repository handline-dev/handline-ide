import { describe, expect, it } from 'vitest';
import { nextWord, offsetWord, zoneNote } from '#src/settings/data/schedules.ts';

const CLOCK = 'Each one runs by the clock of the machine Handline runs on';

describe('the zone a schedule runs in', () => {
  it('writes an offset as people do, east and west, with its minutes when it has them', () => {
    expect(offsetWord(540)).toBe('UTC+9');
    expect(offsetWord(0)).toBe('UTC+0');
    expect(offsetWord(-480)).toBe('UTC-8');
    expect(offsetWord(330)).toBe('UTC+5:30');
    expect(offsetWord(-210)).toBe('UTC-3:30');
  });
  it('names the machine clock, and says Next and Last are the reader own time only when the zones differ', () => {
    expect(zoneNote({ name: 'KST', offset_min: 540 }, 540)).toBe(`${CLOCK} (KST, UTC+9).`);
    expect(zoneNote({ name: 'KST', offset_min: 540 }, -480)).toBe(`${CLOCK} (KST, UTC+9). Next and Last are in your own time.`);
  });
  it('says a zone that has no name of its own by its offset once', () => {
    for (const name of ['+09', 'GMT+9', 'UTC+9', '']) expect(zoneNote({ name, offset_min: 540 }, 540)).toBe(`${CLOCK} (UTC+9).`);
    expect(zoneNote({ name: 'UTC', offset_min: 0 }, 0)).toBe(`${CLOCK} (UTC+0).`);
  });
});

describe('when a schedule is sent next', () => {
  const now = new Date(2026, 10, 27, 23, 50).getTime();
  it('says today and tomorrow by the calendar day, on the 24-hour clock', () => {
    expect(nextWord(new Date(2026, 10, 27, 23, 55).toISOString(), now)).toBe('Today 23:55');
    // twenty minutes on, and a day later by the calendar
    expect(nextWord(new Date(2026, 10, 28, 0, 10).toISOString(), now)).toBe('Tomorrow 00:10');
    expect(nextWord(new Date(2026, 10, 28, 17, 5).toISOString(), now)).toBe('Tomorrow 17:05');
  });
  it('names the day from the day after tomorrow on', () => {
    const word = nextWord(new Date(2026, 10, 30, 9, 30).toISOString(), now);
    expect(word).toMatch(/30.* 09:30$/);
    expect(word).not.toMatch(/Today|Tomorrow/);
  });
});
