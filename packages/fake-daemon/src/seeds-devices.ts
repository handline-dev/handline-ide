// Lent machines for Settings › Devices: a Mac on Run with an agent and one it cannot run, a server on Pause, a Mac on another build, a Mac
// offline since a while ago, and another Mac on Off. The master's own Mac stays first, with the seeded phones after.
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

const GB = 1024 ** 3;
const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString();

export const DEVICE_SEEDS: Record<string, Seed> = {
  /** This Mac has become slow under the agents' runs: three of five are late, and one more waits when a policy holds runs. */
  'slow-mac': (d) => void (d.admin.slowMac = true),
  devices: (d) => {
    d.admin.lend({
      id: 'd-studio',
      name: 'Studio',
      kind: 'mac',
      lending: 'run',
      agents: ['handline.term-build'],
      lent: {
        load: { cores: 12, load1: 4.6, ram_total: 64 * GB, ram_free: 29 * GB, agents_rss: 9 * GB },
        battery: false,
        lacks: [{ agent: 'handline.auditor', why: 'its project is not there' }],
      },
    });
    d.admin.lend({
      id: 'd-build',
      name: 'build-server',
      kind: 'server',
      lending: 'pause',
      lent: { load: { cores: 32, load1: 1.2, ram_total: 128 * GB, ram_free: 100 * GB, agents_rss: 0 } },
    });
    d.admin.lend({ id: 'd-laptop', name: 'MacBook', kind: 'mac', lending: 'run', lent: { version: '1a2b3c4' } });
    d.admin.lend({
      id: 'd-mini',
      name: 'old-mini',
      kind: 'mac',
      lending: 'run',
      connected: false,
      lent: { online: false, last_seen: ago(48) },
    });
    d.admin.lend({ id: 'd-air', name: 'Air', kind: 'mac', lending: 'off' });
  },
};
