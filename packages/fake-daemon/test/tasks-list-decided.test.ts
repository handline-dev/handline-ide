import { test } from 'node:test';
import assert from 'node:assert/strict';
import { type RpcError } from '@handline/client';
import { setup } from '#src/helpers.ts';

test('tasks.list order decided pages the decisions newest first, cursor to cursor, with no task twice or left out', async () => {
  const { daemon, client } = setup();
  await client.start();
  const rows = (daemon.tasks as unknown as { rows: { id: number; decided?: { at: string; ask: string; answer: string } | null }[] }).rows;
  for (const r of rows) r.decided = null;
  // An old task decided last comes first; two decided at the same time go by id.
  const at = ['2026-11-01T00:00:03Z', '2026-11-01T00:00:01Z', '2026-11-01T00:00:02Z', '2026-11-01T00:00:02Z', '2026-11-01T00:00:05Z'];
  const picked = rows.slice(0, at.length);
  picked.forEach((r, i) => (r.decided = { at: at[i]!, ask: 'q', answer: 'a' }));
  const want = picked
    .map((r, i) => ({ id: r.id, at: at[i]! }))
    .sort((a, b) => b.at.localeCompare(a.at) || b.id - a.id)
    .map((r) => r.id);
  const got: number[] = [];
  let cursor: string | undefined;
  for (let n = 0; n < 5; n++) {
    const page = await client.call('tasks.list', {
      decided_since: '1970-01-01T00:00:00Z',
      order: 'decided',
      limit: 2,
      ...(cursor ? { cursor } : {}),
    });
    got.push(...page.items.map((t) => t.id));
    assert.equal(page.more, page.next !== undefined);
    if (!page.next) break;
    cursor = page.next;
  }
  assert.deepEqual(got, want);
});

test('tasks.list order decided needs decided_since and pages by cursor only', async () => {
  const { client } = setup();
  await client.start();
  const list = (p: Record<string, unknown>) => client.call('tasks.list', p as never);
  for (const [p, field] of [
    [{ order: 'decided' }, 'order'],
    [{ decided_since: '1970-01-01T00:00:00Z', order: 'decided', before: 5 }, 'before'],
    [{ decided_since: '1970-01-01T00:00:00Z', order: 'decided', ids: [1] }, 'order'],
  ] as const)
    await assert.rejects(list(p), (e: RpcError) => e.data?.field === field);
});
