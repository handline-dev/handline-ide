import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('tools.install runs a missing tool or an uninstalled vendor in a window shell.close ends, and the tool is installed', async () => {
  const { client, daemon } = setup();
  await client.start();
  daemon.admin.missingTools = [{ tool: 'git', fix: 'xcode-select --install' }];
  const git = await client.call('tools.install', { tool: 'git', request_id: 'i1' });
  assert.equal(git.tool, 'git');
  assert.equal((await client.call('daemon.status', {})).missing_tools, undefined);
  const codex = await client.call('tools.install', { tool: 'codex', request_id: 'i2' });
  assert.notEqual(codex.terminal, git.terminal);
  assert.equal((await client.call('daemon.status', {})).vendors?.find((v) => v.vendor === 'codex')?.installed, true);
  await assert.rejects(client.call('tools.install', { tool: 'claude', request_id: 'i3' }), (e: RpcError) => e.data?.what === 'tool');
  await client.call('shell.close', { terminal: git.terminal, request_id: 'i4' });
});
