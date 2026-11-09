import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { PendingRequest, RequestRow } from '@handline/client';
import { bulkNames, bulkOf, pickAllState, requestRow, turnRow, turnsCounted, unseenOf, type Picked } from '#src/index.ts';

const req = (cursor: string, over: Partial<RequestRow> = {}): RequestRow =>
  ({ cursor, task: 1, title: `title ${cursor}`, ended: null, ...over }) as unknown as RequestRow;
const turn = (over: Partial<PendingRequest> = {}): PendingRequest =>
  ({ source: 'task', id: '1', task: 1, items: [], ...over }) as unknown as PendingRequest;
const ended = (seen: boolean) => ({ ended: { seen, outcome: 'done', at: '' } }) as unknown as Partial<RequestRow>;
const rowsOf = (...rs: RequestRow[]): Picked[] => rs.map(requestRow);

test('an action names the rows it acts on by their kind', () => {
  const rows = [
    requestRow(req('open', { task: 5 })),
    requestRow(req('o2', { task: 6 })),
    requestRow(req('new', { ...ended(false), task: 7 })),
    turnRow(turn({ id: 'q', task: 8, kind: 'ask' })),
  ];
  const name = bulkNames(bulkOf(rows, new Set([8])));
  assert.equal(name.cancel, 'Cancel 2 requests');
  assert.equal(name.seen, 'Mark 1 result seen');
  assert.equal(name.giveBack, 'Give 1 turn back');
  assert.equal(turnsCounted([turn({ kind: 'ask' }), turn({ kind: 'ask' })]), '2 questions');
  assert.equal(turnsCounted([turn({ kind: 'ask' }), turn({ kind: 'allow' })]), '2 items');
});

test('only a finished row not seen yet is to mark seen, and only with a task', () => {
  const rows = rowsOf(
    req('open'),
    req('seen', { ...ended(true), task: 2 }),
    req('new', { ...ended(false), task: 3 }),
    req('none', { ...ended(false), task: null }),
  );
  assert.deepEqual(unseenOf(rows), [{ keys: ['request:new'], task: 3 }]);
});

test('only an open request with a task is to cancel: a finished one and a turn are not', () => {
  const rows = [
    ...rowsOf(req('open', { task: 5 }), req('done', { ...ended(false), task: 6 }), req('none', { task: null })),
    turnRow(turn({ task: 7 })),
  ];
  assert.deepEqual(bulkOf(rows, new Set()).cancel, [{ keys: ['request:open'], task: 5, title: 'title open' }]);
});

test('two rows of one task write once, for both rows', () => {
  const b = bulkOf(
    rowsOf(req('a', { task: 5 }), req('b', { task: 5 }), req('c', { ...ended(false), task: 6 }), req('d', { ...ended(false), task: 6 })),
    new Set(),
  );
  assert.deepEqual(
    b.cancel.map((x) => x.keys),
    [['request:a', 'request:b']],
  );
  assert.deepEqual(
    b.seen.map((x) => x.keys),
    [['request:c', 'request:d']],
  );
  assert.equal(bulkNames(b).cancel, 'Cancel 2 requests');
  const turns: Picked[] = [
    { key: 'x', p: turn({ id: '1', task: 9 }) },
    { key: 'y', p: turn({ source: 'comment', id: '2', task: 9 }) },
  ];
  assert.deepEqual(bulkOf(turns, new Set([9])).giveBack, [{ keys: ['x', 'y'], task: 9 }]);
});

test('a turn is given back only when the founder holds its top turn', () => {
  const rows: Picked[] = [{ key: 'x', p: turn({ task: 9 }) }];
  assert.deepEqual(bulkOf(rows, new Set()).giveBack, []);
  assert.deepEqual(bulkOf(rows, new Set([9])).giveBack, [{ keys: ['x'], task: 9 }]);
});

test('the select-all box follows the rows shown, not rows picked and gone', () => {
  const [a, b] = rowsOf(req('a'), req('b')) as [Picked, Picked];
  const picked = (...xs: Picked[]) => new Map(xs.map((x) => [x.key, x]));
  assert.equal(pickAllState([a, b], picked()), 'none');
  assert.equal(pickAllState([a, b], picked(a)), 'some');
  assert.equal(pickAllState([a, b], picked(a, b)), 'all');
  assert.equal(pickAllState([a], picked(b)), 'none');
});
