// Seeds for a turn's steps in the chat: an agent mid-turn with each kind of step.
import type { TranscriptItem } from '@handline/client';
import type { FakeDaemon } from '#src/daemon.ts';

const ARCH = 'handline.architect';

export const STEP_SEEDS: Record<string, (d: FakeDaemon) => void> = {
  /** Tasks given to the architect, one assigned and one handed on, and a turn running 4 minutes on the one it holds (T8705). */
  'start-card': (d) => {
    const iso = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    d.tasks.seed({ id: 1683, from: 'handline', to: ARCH, title: 'Chat plan, second pass', state: 'done' });
    d.chat.seed(ARCH, {
      from: 'handline',
      to: ARCH,
      kind: 'task',
      tasks: [{ id: 1683, title: 'Chat plan, second pass' }],
      body: 'T1683 Chat plan, second pass',
      at: iso(40),
      landing: 'delivered',
    });
    d.chat.seed(ARCH, {
      from: 'handline.ui',
      to: ARCH,
      kind: 'task',
      tasks: [{ id: 1684, title: 'Web UI design' }],
      body: 'T1684 handed to you: Web UI design',
      at: iso(4),
      landing: 'delivered',
    });
    d.tasks.seed({ id: 1685, from: 'handline', to: ARCH, title: 'Pause tests: wait past the until', state: 'queued' });
    d.chat.seedItem(ARCH, {
      at: iso(20),
      kind: 'task_event',
      task_event: { event: 'unblocked', task: 1685, closed_task: 1683, request: null, evidence: null, who: 'handline.auditor' },
    });
    const AGENT_CALL = 900_003; // a tool call, as the daemon numbers it: its subagent's steps name it as their parent
    const steps: [number, Partial<TranscriptItem>][] = [
      [3.9, { kind: 'tool', tool: 'Read', text: 'go/internal/daemon/pause_test.go' }],
      [3.8, { kind: 'thinking', text: 'The until is compared before it is rounded, so a wake at .999 lands a second early.' }],
      [
        3.5,
        {
          kind: 'reply',
          id: 900_000 /* even: a session's own text, as the daemon numbers it */,
          text: 'The check fires 1 ms before the until; running the two tests under load.',
        },
      ],
      [
        3,
        {
          kind: 'tool',
          tool: 'Bash',
          text: 'go test ./internal/daemon -run TestPause -count=30',
          output:
            '--- FAIL: TestPauseUntil (0.00s)\n    pause_test.go:88: woke at 10:00:00, want 10:00:01\nFAIL\nFAIL\thandline/internal/daemon\t0.41s',
        },
      ],
      [
        2.5,
        {
          kind: 'tool',
          tool: 'Edit',
          text: 'go/internal/daemon/pause.go',
          diff: {
            hunks: [
              { old_start: 41, new_start: 41, lines: [' \tnow := clock.Now()', '-\tif now.Before(until) {', '+\tif !now.After(until) {'] },
            ],
            dropped_lines: 0,
          },
          added: 1,
          removed: 1,
        },
      ],
      [
        2.2,
        {
          kind: 'tool',
          id: AGENT_CALL,
          tool: 'Agent',
          text: 'Check the other callers of until',
          output: 'Only pause.go compares before rounding.',
        },
      ],
      [2.1, { kind: 'tool', tool: 'Grep', text: 'Before\\(until', parent: AGENT_CALL, output: 'go/internal/daemon/pause.go:42' }],
      [2, { kind: 'tool', tool: 'Read', text: 'go/internal/daemon/pause.go', parent: AGENT_CALL }],
      [1.5, { kind: 'tool', tool: 'Bash', text: 'pnpm --dir apps/web dev', running: true }],
      [1, { kind: 'tool', tool: 'Bash', text: 'go test ./internal/daemon -run TestPause -count=30', running: true }],
    ];
    for (const [min, it] of steps) d.chat.seedItem(ARCH, { at: iso(min), kind: 'tool', ...it });
    d.chat.setWorking(ARCH, true, 'Working');
    const s = d.sessions.get(ARCH)!;
    s.fx.view = { ...s.fx.view, doing: { text: 'Bash Run the two pause tests 30 times under load', at: iso(1) } };
  },
};
