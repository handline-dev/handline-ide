import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';
import { PLAIN_FOLDER } from '#src/worktrees.ts';

test('folder.init makes the plain folder a repository whose facts then read, and refuses one that already is', async () => {
  const { client } = setup();
  await client.start();
  await assert.rejects(
    client.call('repo.facts', { path: PLAIN_FOLDER }),
    (e: RpcError) => e.code === 'not_found' && e.data?.what === 'path',
  );
  assert.equal((await client.call('folder.init', { path: PLAIN_FOLDER, request_id: 'i1' })).path, PLAIN_FOLDER);
  assert.equal((await client.call('repo.facts', { path: PLAIN_FOLDER })).test_command, 'pnpm test');
  assert.equal((await client.call('repos.recent', {})).folders[0]?.path, PLAIN_FOLDER);
  await assert.rejects(client.call('folder.init', { path: PLAIN_FOLDER, request_id: 'i2' }), (e: RpcError) => e.code === 'refused');
  client.close();
});
