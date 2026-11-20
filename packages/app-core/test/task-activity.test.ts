import { test } from 'node:test';
import assert from 'node:assert/strict';
import { activityOf, addressText, noteText, toldOf } from '#src/task-activity.ts';

const e = (kind: string, who: string, text: string, at: string, memo: number | null = null) =>
  ({ at, who, kind, text, memo, edited_at: null }) as Parameters<typeof activityOf>[1][number];

test('entries become comments and events in words, a handoff one line with its note kept', () => {
  const a = activityOf(
    [{ id: 7, task: 1, author: 'founder', at: '2026-01-01T00:00:03Z', text: 'Hello' }],
    [
      e('state', 'handline.pc', 'new -> opened', '2026-01-01T00:00:01Z'),
      e('handoff', 'handline.pc → worker2', 'Over to you', '2026-01-01T00:00:02Z'),
      e('memo', 'founder', 'Hello', '2026-01-01T00:00:03Z', 7),
      e('state', 'worker2', 'queued -> doing', '2026-01-01T00:00:04Z'),
      e('reject', 'handline.pc', 'Not yet', '2026-01-01T00:00:05Z'),
    ],
  );
  assert.deepEqual(
    a.map((i) => [i.kind, i.line]),
    [
      ['event', 'handline.pc opened it'],
      ['event', 'handline.pc handed it to worker2'],
      ['comment', ''],
      ['event', 'worker2 started it'],
      ['event', 'handline.pc sent it back'],
    ],
  );
  assert.equal(a[1]!.note, 'Over to you');
  assert.equal(a[2]!.memo, 7);
});

test('a comment lists the files its text does not name, a text file too; one it names is drawn in place, not again', () => {
  const memo = {
    id: 1,
    author: 'human',
    at: '2026-11-06T00:00:00Z',
    text: 'look ![capture](attachment:aaa)',
    files: [
      { id: 'aaa', name: 'capture.png', kind: 'image', mime: 'image/png', bytes: 10, gone: false },
      { id: 'bbb', name: 'other.png', kind: 'image', mime: 'image/png', bytes: 20, gone: false },
      { id: 'ccc', name: 'notes.md', kind: 'markdown', mime: 'text/markdown', bytes: 30, gone: false },
    ],
  } as unknown as Parameters<typeof activityOf>[0][number];
  assert.deepEqual(activityOf([memo], [])[0]!.images, [
    { id: 'bbb', name: 'other.png', mime: 'image/png', size: 20 },
    { id: 'ccc', name: 'notes.md', mime: 'text/markdown', size: 30 },
  ]);
});

test('a turn passed or a handoff reads by who it went to, the person as you; the person giving it back is no line; a delivery the daemon made says why', () => {
  const turn = (who: string, text: string, at: string) => ({ ...e('turn', who, text, at), turn: text ? 'push' : 'pop' }) as const;
  const a = activityOf(
    [],
    [
      turn('handline.ui → human', 'Which one?', '2026-01-01T00:00:01Z'),
      turn('human → handline.ui', '', '2026-01-01T00:00:02Z'),
      turn('handline.ui → handline.pc', 'Look at this', '2026-01-01T00:00:03Z'),
      { ...e('turn', 'handline.pc → human', '', '2026-01-01T00:00:03Z'), turn: 'push' },
      e('handoff', 'handline.pc → human', 'Pick one', '2026-01-01T00:00:03Z'),
      { ...e('request', 'handline.pc → handline.ui', 'landed  T1', '2026-01-01T00:00:04Z'), forwards: 'queue' },
      { ...e('request', 'handline.pc → handline.ux', 'landed  T1', '2026-01-01T00:00:05Z'), forwards: 'resend' },
    ],
    'human',
  );
  assert.deepEqual(
    a.map((i) => [i.line, i.note, i.turnTo]),
    [
      ['handline.ui asked you', 'Which one?', 'human'],
      ['handline.ui gave the turn to handline.pc', 'Look at this', 'handline.pc'],
      ['handline.pc asked you', '', 'human'],
      ['handline.pc assigned you', 'Pick one', undefined],
      ['It reached handline.ui from the queue', 'landed  T1', undefined],
      ['It went again to handline.ux, its new holder', 'landed  T1', undefined],
    ],
  );
});

test("the person's comment that gives the turn back is headed with whom it went to", () => {
  const memo = { id: 7, author: 'human', at: '2026-01-01T00:00:02Z', text: 'Go.' } as unknown as Parameters<typeof activityOf>[0][number];
  const [c, ...rest] = activityOf(
    [memo],
    [{ ...e('turn', 'human → handline.ui', '', '2026-01-01T00:00:02Z'), memo: 7, turn: 'pop' }],
    'human',
  );
  assert.deepEqual([c!.turn, c!.turnTo, rest.length], ['pop', 'handline.ui', 0]);
});

test('addressText drops a leading mention of whom the comment went to, by id or name, and keeps others', () => {
  assert.equal(addressText('@architect what is it?', ['a2', 'architect']), 'what is it?');
  assert.equal(addressText('@a2 what is it?', ['a2', 'architect']), 'what is it?');
  assert.equal(addressText('@lead ask @architect', ['a2', 'architect']), '@lead ask @architect');
  assert.equal(addressText('see @architect', ['architect']), 'see @architect');
  assert.equal(addressText('<@architect> what is it?', ['a2', 'architect']), 'what is it?');
  assert.equal(addressText('<@lead> ask', ['a2', 'architect']), '<@lead> ask');
  assert.equal(noteText('@human OK?'), 'OK?');
});

const told = (agent: string) => ({ agent, request: 1, state: 'sent' as const, at: '2026-11-21T00:00:00Z' });
test("the founder's own comment is addressed to whom the daemon told, never the founder", () => {
  assert.deepEqual(toldOf({ readers: [told('human'), told('architect')] }, 'ron'), ['architect']);
  assert.deepEqual(toldOf({ readers: [told('ron')] }, 'ron'), []);
  assert.deepEqual(toldOf({}, 'ron'), []);
  assert.equal(addressText('@architect here', toldOf({ readers: [told('architect')] }, 'ron')), 'here');
});
