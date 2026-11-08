import { test } from 'node:test';
import assert from 'node:assert/strict';
import { second, setup } from '#src/helpers.ts';

const AGENT = 'handline.architect';

test('a comment asks nothing; the founder is asked by a turn, and asking again rewrites it', async () => {
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
        edited: string | null;
        earlier?: { ask: string }[];
      }[]
    ).at(-1);
  await worker.call('task.turn.push', { task, to: 'human', ask: 'A or B?', request_id: 'a3' });
  await worker.call('task.turn.push', { task, to: 'human', ask: 'A or B?', request_id: 'a4' });
  assert.deepEqual([(await turn())?.agent, (await turn())?.ask, (await turn())?.edited], ['human', 'A or B?', null]);
  await worker.call('task.turn.push', { task, to: 'human', ask: 'A, B or C?', request_id: 'a5' });
  const t = await turn();
  assert.deepEqual([t?.ask, t?.earlier?.map((e) => e.ask), !!t?.edited], ['A, B or C?', ['A or B?'], true]);
  worker.close();
  s.client.close();
});
