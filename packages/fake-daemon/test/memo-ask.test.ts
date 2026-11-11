import { test } from 'node:test';
import assert from 'node:assert/strict';
import { second, setup } from '#src/helpers.ts';

const AGENT = 'handline.architect';

test('a comment asks nothing; the founder is asked by a comment with a turn, and a new one moves the turn to it', async () => {
  const s = setup({ client: 'boss' });
  s.daemon.tasks.verbs = true;
  await s.client.start();
  const worker = second(s.daemon, s.net, { client: AGENT });
  await worker.start();
  const task = (await s.client.call('task.new', { owner: AGENT, title: 'a task', body: 'build it', request_id: 'a1' })).task as number;
  const asks = async () =>
    ((await s.client.call('inbox.list', { unread: true })).items as { kind: string; memo?: number }[]).filter((i) => i.memo !== undefined);
  await worker.call('task.comment.add', { task, text: '@human which one?', request_id: 'a2' });
  assert.deepEqual(await asks(), []);
  const turn = async () =>
    (
      (await s.client.call('task.get', { task })).turns as {
        agent: string;
        ask: string;
        memo: number | null;
        edited: string | null;
        earlier?: { ask: string }[];
      }[]
    ).at(-1);
  const first = (await worker.call('task.comment.add', { task, text: 'A or B?', turn: 'human', request_id: 'a3' })).memo;
  const u = await turn();
  assert.deepEqual([u?.agent, u?.ask, u?.memo, u?.edited], ['human', 'A or B?', first, null]);
  const second_ = (await worker.call('task.comment.add', { task, text: 'A, B or C?', turn: 'human', request_id: 'a5' })).memo;
  const t = await turn();
  assert.deepEqual([t?.ask, t?.memo, t?.earlier?.map((e) => e.ask), !!t?.edited], ['A, B or C?', second_, ['A or B?'], true]);
  worker.close();
  s.client.close();
});
