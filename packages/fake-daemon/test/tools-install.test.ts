import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';
import { applySeeds } from '#src/seeds.ts';

test('tools.install runs a missing tool or an uninstalled vendor in a window that closes once the tool is installed and stays when it fails', async () => {
  const { client, daemon } = setup();
  await client.start();
  daemon.admin.missingTools = [{ tool: 'git', fix: 'xcode-select --install' }];
  const git = await client.call('tools.install', { tool: 'git', request_id: 'i1' });
  assert.equal(git.tool, 'git');
  const status = await client.call('daemon.status', {});
  assert.deepEqual(status.missing_tools, [
    { tool: 'git', fix: 'xcode-select --install', install: { terminal: git.terminal, ended: false } },
  ]);
  assert.equal((await client.call('tools.install', { tool: 'git', request_id: 'i2' })).terminal, git.terminal);
  daemon.endInstall('git');
  assert.equal((await client.call('daemon.status', {})).missing_tools, undefined);
  const codex = await client.call('tools.install', { tool: 'codex', request_id: 'i3' });
  assert.notEqual(codex.terminal, git.terminal);
  daemon.endInstall('codex', false);
  const failed = (await client.call('daemon.status', {})).vendors?.find((v) => v.vendor === 'codex');
  assert.deepEqual(failed?.install, { terminal: codex.terminal, ended: true });
  const again = await client.call('tools.install', { tool: 'codex', request_id: 'i4' });
  assert.notEqual(again.terminal, codex.terminal);
  daemon.endInstall('codex');
  assert.equal((await client.call('daemon.status', {})).vendors?.find((v) => v.vendor === 'codex')?.installed, true);
  await assert.rejects(client.call('tools.install', { tool: 'claude', request_id: 'i5' }), (e: RpcError) => e.data?.what === 'tool');
  await assert.rejects(client.call('shell.close', { terminal: git.terminal, request_id: 'i6' })); // it closed when git came
});

test('the installing-git seed has git missing and its install running, which shell.close takes away', async () => {
  const { client, daemon } = setup();
  await client.start();
  applySeeds(daemon, 'installing-git');
  const [git] = (await client.call('daemon.status', {})).missing_tools ?? [];
  assert.equal(git?.install?.ended, false);
  await client.call('shell.close', { terminal: git!.install!.terminal, request_id: 'c1' });
  assert.equal((await client.call('daemon.status', {})).missing_tools?.[0]?.install, undefined);
});
