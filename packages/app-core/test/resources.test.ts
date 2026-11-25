import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Resources } from '@handline/client';
import { byLoad, bytesText, loadFull, memoryLow, useRows } from '#src/index.ts';

const GB = 2 ** 30;
const res = (m: Partial<Resources['machine']> = {}, agents: Resources['agents'] = {}): Resources => ({
  at: '2026-10-19T00:00:00Z',
  agents,
  agents_total: { cpu_pct: 0, rss_bytes: 0 },
  start_hold: null,
  machine: {
    ram_total_bytes: 32 * GB,
    ram_used_bytes: 17 * GB,
    ram_free_bytes: 15 * GB,
    load_1m: 3,
    cores: 10,
    load_per_core: 0.3,
    ram_free_pct: 47,
    disk_free_bytes: 3.6 * GB,
    disk_total_bytes: 500 * GB,
    ...m,
  },
});

test('bytes read as the shortest figure', () => {
  assert.deepEqual([3.6 * GB, 120 * GB, 420 * 2 ** 20].map(bytesText), ['3.6 GB', '120 GB', '420 MB']);
});

test('agents are heaviest first with a totals line', () => {
  const { rows, total } = useRows(
    res(
      {},
      {
        a: { cpu_pct: 1, rss_bytes: 100 * 2 ** 20 },
        b: { cpu_pct: 95, rss_bytes: 400 * 2 ** 20 },
        c: { cpu_pct: 1, rss_bytes: 300 * 2 ** 20 },
      },
    ),
  );
  assert.deepEqual(
    rows.map((r) => r.agent),
    ['b', 'c', 'a'],
  );
  assert.equal(total, '3 agents · CPU 97% · memory 800 MB');
});

test('a limit of never holds nothing, and the memory floor holds under it', () => {
  const r = res({ load_per_core: 5, ram_free_pct: 8 });
  assert.equal(loadFull(r, 0), false);
  assert.equal(loadFull(r, 150), true);
  assert.equal(memoryLow(r, 10), true);
  assert.equal(memoryLow(r, 0), false);
});

test('agents are listed heaviest first: CPU, then memory, then name', () => {
  const rows = [
    { agent: 'b', cpu_pct: 5, rss_bytes: 100 },
    { agent: 'a', cpu_pct: 90, rss_bytes: 10 },
    { agent: 'd', cpu_pct: 5, rss_bytes: 900 },
    { agent: 'c', cpu_pct: 5, rss_bytes: 100 },
  ];
  assert.deepEqual(
    byLoad(rows).map((r) => r.agent),
    ['a', 'd', 'b', 'c'],
  );
});
