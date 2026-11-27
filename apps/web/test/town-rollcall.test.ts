import { describe, expect, it } from 'vitest';
import type { Activity } from '#src/layout/agentList.ts';
import type { House, Person } from '#src/town/model.ts';
import { countsOf, doneToday, groupWords, rollCallOf, teamLine } from '#src/town/rollcall.ts';

const person = (key: string, activity: Activity): Person => ({ key, name: key, activity, doing: null, queue: 0, cached: false });
const house = (key: string, ...people: Person[]): House => ({ key, name: key, lead: null, people, working: 0, decisions: [] });

describe('the town roll call', () => {
  it('groups everyone by what they are doing, house by house, and leaves out the resting and the asleep', () => {
    const shop = house('shop', person('web', 'working'), person('copy', 'you'), person('design', 'resting'));
    const data = house('data', person('etl', 'error'), person('ml', 'waking'), person('ops', 'held'), person('old', 'asleep'));
    const call = rollCallOf([shop, data]);
    expect(call.working.map((c) => `${c.house.key}/${c.person.key}`)).toEqual(['shop/web', 'data/ml']);
    expect(call.you.map((c) => c.person.key)).toEqual(['copy']);
    expect(call.error.map((c) => c.person.key)).toEqual(['etl']);
    expect(call.held.map((c) => c.person.key)).toEqual(['ops']);
  });
  it("counts one team's groups in the strip's order, leaving out a group with nobody", () => {
    const shop = house(
      'shop',
      person('a', 'you'),
      person('b', 'working'),
      person('c', 'waking'),
      person('d', 'held'),
      person('e', 'asleep'),
    );
    expect(countsOf(shop)).toEqual([
      ['working', 2],
      ['you', 1],
      ['held', 1],
    ]);
    expect(countsOf(house('quiet', person('f', 'resting')))).toEqual([]);
  });
  it("says a team's counts in words, or that nobody is at work", () => {
    expect(teamLine(house('shop', person('a', 'working'), person('b', 'you'), person('c', 'error')))).toBe(
      '1 working · 1 waiting for you · 1 stopped',
    );
    expect(teamLine(house('quiet', person('f', 'asleep')))).toBe('Nobody at work');
  });
  it('says a count in words', () => {
    expect(groupWords('working', 0)).toBe('0 working');
    expect(groupWords('you', 2)).toBe('2 waiting for you');
    expect(groupWords('error', 1)).toBe('1 stopped');
    expect(groupWords('held', 3)).toBe('3 on hold');
  });
});

describe('the work finished today', () => {
  const now = new Date(2030, 4, 6, 15, 0).getTime();
  const at = (h: number, day = 6) => new Date(2030, 4, day, h, 0).toISOString();
  const task = (id: number, state: string, done: string | null) => ({ id, title: `t${id}`, to: 'a1', state, done });
  it('keeps what was done since midnight, newest first, and nothing discarded, open or from another day', () => {
    const tasks = [
      task(1, 'done', at(9)),
      task(2, 'done', at(14)),
      task(3, 'done', at(23, 5)),
      task(4, 'discarded', at(10)),
      task(5, 'doing', null),
      task(6, 'done', at(0)),
    ];
    expect(doneToday(tasks, now).map((t) => t.id)).toEqual([2, 1, 6]);
  });
});
