import { describe, expect, it } from 'vitest';
import { AIM_PAGES, aimStep } from '#src/chat/aim.ts';

const at = '2026-11-20T09:30:00Z';
const reading = { at, hasOlder: true, loadingOlder: false, pages: 0 };

describe('a chat asked to show a message that is not among its lines', () => {
  it('reads an older page while the lines start after the message', () => {
    expect(aimStep({ ...reading, floor: '2026-11-25T00:00:00Z' })).toBe('older');
  });
  it('waits for the page being read before asking for another', () => {
    expect(aimStep({ ...reading, floor: '2026-11-25T00:00:00Z', loadingOlder: true })).toBe('wait');
  });
  it('says the message is gone once the lines reach back to its time, or to the first one', () => {
    expect(aimStep({ ...reading, floor: '2026-11-20T09:30:00Z' })).toBe('gone');
    expect(aimStep({ ...reading, floor: '2026-11-19T00:00:00.000Z' })).toBe('gone');
    expect(aimStep({ ...reading, floor: null, hasOlder: false })).toBe('gone');
  });
  it('gives up on a message further back than it reads, and when no older page is left', () => {
    expect(aimStep({ ...reading, floor: '2026-11-25T00:00:00Z', pages: AIM_PAGES })).toBe('far');
    expect(aimStep({ ...reading, floor: '2026-11-25T00:00:00Z', pages: AIM_PAGES - 1 })).toBe('older');
    expect(aimStep({ ...reading, floor: '2026-11-25T00:00:00Z', hasOlder: false })).toBe('far');
  });
});
