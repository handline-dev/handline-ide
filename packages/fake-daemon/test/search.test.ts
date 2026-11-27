import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { second, setup } from '#src/helpers.ts';
import { snippetOf } from '#src/search.ts';

const ARCH = 'handline.architect'; // local-ref-ok: a fixture agent name

test('search finds a task by its title, brief and evidence, a comment and a message, each with what opens it', async () => {
  const { daemon, client } = setup();
  await client.start();
  const at = '2026-10-01T00:00:00.000Z';
  daemon.tasks.seed({ id: 9101, from: 'human', to: ARCH, title: 'Quince in the title', state: 'queued', created: at });
  daemon.tasks.seed({ id: 9102, from: 'human', to: ARCH, title: 'By its brief', state: 'queued', body: 'Peel the QUINCE first.' });
  daemon.tasks.seed({ id: 9103, from: 'human', to: ARCH, title: 'By its end', state: 'done', evidence: 'abc1234 quince landed' });
  daemon.tasks.seed({
    id: 9104,
    from: 'human',
    to: ARCH,
    title: 'Commented',
    state: 'queued',
    comments: [{ id: 77, task: 9104, author: ARCH, at, text: 'Agreed: quince, not pear.' }],
  });
  const said = daemon.chat.seed(ARCH, { body: 'Is the quince ripe?', from: 'human', to: ARCH, at });
  const { hits, more } = await client.call('search', { query: 'quince' });
  assert.equal(more, false);
  assert.deepEqual(
    hits.map((h) => [h.kind, h.task ?? h.request, h.field ?? h.comment ?? h.agent, h.snippet]),
    [
      ['task', 9103, 'result', 'abc1234 quince landed'],
      ['task', 9102, 'brief', 'Peel the QUINCE first.'],
      ['task', 9101, 'title', 'Quince in the title'],
      ['comment', 9104, 77, 'Agreed: quince, not pear.'],
      ['message', said, ARCH, 'Is the quince ripe?'],
    ],
  );
  assert.deepEqual([hits[3].title, hits[3].from, hits[4].scope, hits[4].from], ['Commented', ARCH, 'mine', 'human']);
});

test('search bounds its hits per kind and says when there are more', async () => {
  const { daemon, client } = setup();
  await client.start();
  for (const id of [9201, 9202, 9203]) daemon.tasks.seed({ id, from: 'human', to: ARCH, title: `medlar ${id}`, state: 'queued' });
  const two = await client.call('search', { query: 'medlar', limit: 2 });
  assert.deepEqual([two.hits.map((h) => h.task), two.more], [[9203, 9202], true]);
  const three = await client.call('search', { query: 'medlar', limit: 3 });
  assert.deepEqual([three.hits.length, three.more], [3, false]);
});

test('search refuses an empty or overlong query, a limit out of range, and an agent', async () => {
  const s = setup();
  await s.client.start();
  for (const query of ['', '   ', 'x'.repeat(201), 7 as never])
    await assert.rejects(s.client.call('search', { query }), (e: RpcError) => e.data?.field === 'query');
  for (const limit of [0, 51, 1.5, '3' as never])
    await assert.rejects(s.client.call('search', { query: 'a', limit }), (e: RpcError) => e.data?.field === 'limit');
  assert.equal((await s.client.call('search', { query: '가'.repeat(200) })).hits.length, 0);
  const agent = second(s.daemon, s.net, { role: 'agent', client: ARCH });
  await agent.start();
  await assert.rejects(agent.call('search', { query: 'a' }), (e: RpcError) => e.data?.gate === 'role');
});

test('a snippet is one line around the match, cut to 160 characters with its cuts marked', () => {
  const pad = '가나다라 '.repeat(60);
  assert.equal(snippetOf('one\ntwo  needle', 'needle'), 'one two needle');
  const start = snippetOf(`needle ${pad}`, 'needle');
  const middle = snippetOf(`${pad}the needle is here ${pad}`, 'NEEDLE');
  const end = snippetOf(`${pad}the needle`, 'needle');
  for (const s of [start, middle, end]) assert.ok([...s].length <= 160, s);
  assert.ok(start.startsWith('needle 가나다라') && start.endsWith('…'));
  assert.ok(middle.startsWith('…') && middle.endsWith('…') && middle.includes('the needle is here'));
  assert.ok(end.startsWith('…') && end.endsWith('the needle'));
});
