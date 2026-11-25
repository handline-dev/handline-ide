import { describe, expect, it } from 'vitest';
import { failedLine, look } from '#src/ui/taskLook.tsx';

const done = (evidence: string | null) => ({ state: 'done', block_reason: null, hold_reason: null, evidence, title: 'Audit' });

describe('a done task whose evidence says FAIL', () => {
  it("reads as failed, with the checker's first line", () => {
    expect(failedLine('FAIL @abc1234 two tests red\nmore')).toBe('Failed: two tests red');
    expect(look(done('FAIL @abc1234 two tests red')).word).toBe('Failed');
  });
  it('is done when the evidence passes or says nothing', () => {
    expect(failedLine('PASS @abc1234 ok')).toBeNull();
    expect(look(done(null)).word).toBe('Done');
    expect(look(done('merged abc')).word).toBe('Done');
  });
});

describe('the hand is for the founder alone', () => {
  const open = (state: string, waiting: { on: string } | null, block: string | null = null) => ({
    state,
    waiting,
    block_reason: block,
    hold_reason: null,
  });
  it('a task waiting on the founder shows the hand', () => expect(look(open('doing', { on: 'founder' })).word).toBe('Waiting'));
  it('a queued task waiting on an agent or a task reads as queued', () => {
    expect(look(open('queued', { on: 'agent' })).word).toBe('Up next');
    expect(look(open('queued', { on: 'task' })).word).toBe('Up next');
  });
  it('a task under way waiting on another reads as blocked', () => {
    expect(look(open('doing', { on: 'task' })).word).toBe('Blocked');
    expect(look(open('doing', null, 'disk low')).word).toBe('Blocked');
  });
});

it("a task on the founder's turn shows the hand", () =>
  expect(look({ state: 'doing', block_reason: null, hold_reason: null, turns: [{}], acting: 'human' }).word).toBe('Waiting'));
const doing = { state: 'doing', block_reason: null, hold_reason: null };

it("a person's turn on top reads as Waiting, never Doing", () => {
  expect(look({ ...doing, turns: [{}], acting: 'p1' }).word).toBe('Waiting');
  expect(look({ ...doing, turns: [{}], acting: 'human' }).word).toBe('Waiting');
});

it("an agent's turn on top still reads as Doing", () => {
  expect(look({ ...doing, turns: [{}], acting: 'handline.architect' }).word).toBe('Doing');
  expect(look({ ...doing, acting: 'p1' }).word).toBe('Doing');
});
