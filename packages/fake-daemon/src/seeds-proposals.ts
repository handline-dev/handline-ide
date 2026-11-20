// Seeds for the proposals and an agent's own handoff prompt.
import type { FakeDaemon } from '#src/daemon.ts';

type Seed = (d: FakeDaemon) => void;

const ARCH = 'handline.architect';
const AUD = 'handline.auditor';

export const PROPOSAL_SEEDS: Record<string, Seed> = {
  /** Three proposals: a new agent (team, model), a setting change old -> new, and one to reject. */
  proposals: (d) => {
    const book = d.script.proposals;
    book.raise({
      id: 901,
      kind: 'add',
      agent: 'handline.new-builder',
      title: 'add handline.new-builder',
      diff: 'add agent handline.new-builder\nteam: handline\nmodel: sonnet',
      asked: { vendor: 'claude', model: 'sonnet', team: 'handline' },
      reason: 'The story has nine independent UI tasks and one builder.',
    });
    book.raise({
      id: 902,
      kind: 'settings',
      agent: ARCH,
      title: `settings ${ARCH}`,
      diff: 'model: sonnet -> opus',
      asked: { sets: { model: 'opus' } },
      reason: 'Design reviews need the stronger model.',
    });
    book.raise({
      id: 903,
      kind: 'retire',
      agent: 'handline.term-build',
      title: 'retire handline.term-build',
      diff: 'retire agent handline.term-build',
      reason: 'Its work is merged. Reject this one to keep the agent.',
    });
  },

  /** Two agents set their own handoff prompt, so the global one is ignored by them. */
  'handoff-own': (d) => {
    d.admin.handoffOwn.push('handline.auditor', 'handline.term-build');
  },

  /** Two more proposals: one applying, held for the agent's running turn, and one the founder asked for themself (`requested_by` human). */
  'proposal-held': (d) => {
    const book = d.script.proposals;
    book.raise({
      id: 904,
      kind: 'settings',
      agent: 'handline.auditor',
      title: 'settings handline.auditor',
      diff: 'effort: medium -> high',
      asked: { sets: { effort: 'high' } },
      state: 'applying',
      waiting_on: ['handline.auditor'],
    });
    book.raise({
      id: 905,
      kind: 'retire',
      agent: 'handline.term-build',
      title: 'retire handline.term-build',
      diff: 'retire agent handline.term-build',
      requested_by: 'human',
      reason: 'Held until its running session ends.',
    });
  },

  /** An approved proposal that did not apply, the daemon's error in its own words, and the daemon down notice's page. */
  'proposal-apply-failed': (d) => {
    d.script.proposals.raise({
      id: 908,
      kind: 'settings',
      agent: ARCH,
      title: 'settings handline.architect',
      diff: 'effort: high',
      state: 'apply_failed',
      error:
        'proposal p908 was not applied: agent.yaml is locked by another process. Retry when the lock clears (/Users/x/.handline/agents/handline.architect/agent.yaml).', // local-ref-ok
    });
  },

  /** Two approved proposals whose change is applied but whose agent did not start: the architect's waits for the load, the auditor's failed. */
  'proposal-followup': (d) => {
    const book = d.script.proposals;
    book.raise({
      id: 906,
      kind: 'settings',
      agent: ARCH,
      title: 'rename handline.architect',
      diff: 'rename handline.architect -> handline.planner',
      state: 'approved',
      settled: new Date().toISOString(),
      follow_up: { agent: ARCH, state: 'waiting', text: 'held: machine load - load 3.09 per core, limit 1.50' },
    });
    book.raise({
      id: 907,
      kind: 'settings',
      agent: AUD,
      title: 'rename handline.auditor',
      diff: 'rename handline.auditor -> handline.reviewer',
      state: 'approved',
      settled: new Date().toISOString(),
      follow_up: { agent: AUD, state: 'failed', text: 'start failed: no free account' },
    });
  },
};
