import { test } from 'node:test';
import assert from 'node:assert/strict';
import { activityOf, foldEvents, noteHead, noteIsLong, NOTE_LINES } from '#src/task-activity.ts';

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

test('only the last three events show until the rest are asked for, comments always', () => {
  const a = activityOf(
    [{ id: 1, task: 1, author: 'y', at: '2026-01-01T00:00:00Z', text: 'hi' }],
    [1, 2, 3, 4, 5].map((n) => e('state', 'x', 'a -> doing', `2026-01-01T00:00:0${n}Z`)),
  );
  const shut = foldEvents(a, false);
  assert.equal(shut.hidden, 2);
  assert.equal(shut.items.length, 4);
  assert.equal(shut.items.filter((i) => i.kind === 'comment').length, 1);
  assert.deepEqual([foldEvents(a, true).hidden, foldEvents(a, true).items.length], [0, 6]);
});

test('noteHead cuts by lines and by characters at a word, and leaves a short note whole', () => {
  assert.equal(noteHead('short\nnote'), 'short\nnote');
  const lines = Array.from({ length: 12 }, (_, i) => `line ${i + 1}`).join('\n');
  assert.equal(noteHead(lines).split('\n').length, NOTE_LINES);
  const para = Array.from({ length: 140 }, (_, i) => `word${i}`).join(' ');
  assert.ok(noteIsLong(para));
  const head = noteHead(para);
  assert.ok(head.length <= NOTE_LINES * 80 + 1 && head.endsWith('…') && !head.endsWith(' …'));
  assert.ok(para.startsWith(head.slice(0, -1)) && /^word\d+$/.test(head.slice(0, -1).split(' ').pop()!));
});

test('a comment lists the image files its text does not name; one it names is drawn in place, not again', () => {
  const memo = {
    id: 1,
    author: 'human',
    at: '2026-11-06T00:00:00Z',
    text: 'look ![capture](attachment:aaa)',
    files: [
      { id: 'aaa', name: 'capture.png', kind: 'image', gone: false },
      { id: 'bbb', name: 'other.png', kind: 'image', gone: false },
    ],
  } as unknown as Parameters<typeof activityOf>[0][number];
  assert.deepEqual(activityOf([memo], [])[0]!.images, [{ id: 'bbb', name: 'other.png' }]);
});

test('a turn passed or a handoff reads by who it went to, the person as you; a delivery the daemon made says why', () => {
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
      ['You gave the turn back to handline.ui', '', 'handline.ui'],
      ['handline.ui gave the turn to handline.pc', 'Look at this', 'handline.pc'],
      ['handline.pc asked you', '', 'human'],
      ['handline.pc assigned you', 'Pick one', undefined],
      ['It reached handline.ui from the queue', 'landed  T1', undefined],
      ['It went again to handline.ux, its new holder', 'landed  T1', undefined],
    ],
  );
});
