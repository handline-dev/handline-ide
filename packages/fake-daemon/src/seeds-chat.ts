// Seeds for the chat screen.
import { crc32, deflateSync } from 'node:zlib';
import type { TranscriptItem } from '@handline/client';
import type { FakeDaemon } from '#src/daemon.ts';
import { task } from '#src/seedkit.ts';

type Seed = (d: FakeDaemon) => void;

const ARCH = 'handline.architect';
const ME = 'human';

/** A one-colour PNG of the size given: a picture with a real height for a layout check. */
function solidPng(w: number, h: number): Buffer {
  const chunk = (type: string, data: Buffer) => {
    const out = Buffer.concat([Buffer.alloc(4), Buffer.from(type), data, Buffer.alloc(4)]);
    out.writeUInt32BE(data.length, 0);
    out.writeUInt32BE(crc32(out.subarray(4, out.length - 4)), out.length - 4);
    return out;
  };
  const head = Buffer.alloc(13);
  head.writeUInt32BE(w, 0);
  head.writeUInt32BE(h, 4);
  head.set([8, 2, 0, 0, 0], 8);
  const row = Buffer.concat([Buffer.from([0]), Buffer.alloc(w * 3, 0x9a)]);
  return Buffer.concat([
    Buffer.from('89504e470d0a1a0a', 'hex'),
    chunk('IHDR', head),
    chunk('IDAT', deflateSync(Buffer.concat(Array.from({ length: h }, () => row)))),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}

export const CHAT_SEEDS: Record<string, Seed> = {
  /** Two restarts back to back for changed team rules, as the daemon words them: the chat shows one line. */
  restarts: (d) => {
    const iso = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    const changed = 'team rules changed (shared/team/handline.md)';
    for (const [n, min] of [
      [1, 4],
      [2, 3],
    ] as const)
      d.chat.seedItem(ARCH, {
        at: iso(min),
        kind: 'session',
        session: { id: `r${n}`, started: iso(min), handoff: null, resumed: true, cause: 'settings_applied', changed },
      });
  },
  /** A restart for team instructions and rules under way after one that is done, in the daemon's words (T9059): the done one is one short
   * line, the live one sits in the turn's row at the chat's foot. */
  restarting: (d) => {
    const at = new Date(Date.now() - 3 * 60_000).toISOString();
    const team = 'team instructions changed (team.yaml, the agent list or a Handline update)';
    d.chat.seedItem(ARCH, {
      at,
      kind: 'session',
      session: {
        id: 'r1',
        started: at,
        handoff: null,
        resumed: true,
        cause: 'restart',
        changed: `${team}, team rules changed, ${team}, team rules changed (workspaces/w1/base.md)`,
      },
    });
    const s = d.sessions.get(ARCH)!;
    s.fx.view = {
      ...s.fx.view,
      handoff: {
        cause: 'settings_applied',
        phase: 'waiting',
        since: new Date().toISOString(),
        settings: ['team instructions', 'team rules'],
      },
    };
  },
  /** A person's move kept for the end of the turn, after one that landed without a note and one that could not run (T8905). */
  'move-scheduled': (d) => {
    const iso = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    d.chat.seedItem(ARCH, {
      at: iso(30),
      kind: 'account_moved',
      account_moved: { from: 'alpha', to: 'beta', cause: 'manual', note_written: false },
    });
    d.chat.seedItem(ARCH, { at: iso(10), kind: 'move_failed', move_failed: { to: 'gamma', reason: 'gamma is at its usage limit' } });
    const s = d.sessions.get(ARCH)!;
    s.fx.view = { ...s.fx.view, pending_move: { to: 'alpha', since: iso(1), phase: 'scheduled' } };
  },
  /** ! lines (T8684). */
  bash: (d) => {
    const iso = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    const line = (min: number, body: string, item: Partial<TranscriptItem>) => {
      const request = d.chat.seed(ARCH, { from: ME, to: ARCH, body: `!${body}`, at: iso(min), landing: 'delivered' });
      d.chat.seedItem(ARCH, { at: iso(min), kind: 'bash', command: body, request, ...item });
    };
    line(9, 'git log --oneline -3', {
      done: true,
      stdout: 'd3d1fa0 Requests\ncd13127 Removes step 49\ne49eecd Hard rule',
      dropped_lines: 12,
    });
    line(8, 'ls /no/such/path', { done: true, stderr: 'ls: /no/such/path: No such file or directory' });
    line(7, 'true', { done: true });
    d.chat.seed(ARCH, {
      from: ME,
      to: ARCH,
      body: '!npm test',
      at: iso(6),
      landing: 'delivered',
      receipt: { state: 'unknown', uuid: 'seed-bash-lost' },
    });
    d.chat.seedItem(ARCH, { at: iso(5), kind: 'bash', command: 'pwd', done: true, stdout: '/Users/user/work' });
    line(1, 'sleep 600', { done: false });
  },

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
    const steps: [number, Partial<TranscriptItem>][] = [
      [3.9, { kind: 'tool', tool: 'Read', text: 'go/internal/daemon/pause_test.go' }],
      [
        3.5,
        {
          kind: 'reply',
          id: 900_000 /* even: a session's own text, as the daemon numbers it */,
          text: 'The check fires 1 ms before the until; running the two tests under load.',
        },
      ],
      [3, { kind: 'tool', tool: 'Bash', text: 'Run the two pause tests 30 times under load' }],
      [2, { kind: 'tool', tool: 'Edit', text: 'go/internal/daemon/pause.go' }],
      [1, { kind: 'tool', tool: 'Bash', text: 'Run the two pause tests 30 times under load' }],
    ];
    for (const [min, it] of steps) d.chat.seedItem(ARCH, { at: iso(min), kind: 'tool', ...it });
    d.chat.setWorking(ARCH, true, 'Working');
    const s = d.sessions.get(ARCH)!;
    s.fx.view = { ...s.fx.view, doing: { text: 'Bash Run the two pause tests 30 times under load', at: iso(1) } };
  },

  /** Chat messages with one-tap actions: OK / No on an open task the founder owns. */
  'chat-actions': (d) => {
    const at = Date.now();
    const iso = (min: number) => new Date(at + min * 60_000).toISOString();
    task(d, 6902, {
      id: 6902,
      from: ARCH,
      to: ME,
      title: 'Open task the founder owns',
      body: 'The plan has three steps: the chat first, then the task page, then Usage.\n\nSay if the order should change.',
      state: 'doing',
      priority: 2,
      time_box: '1h',
      clock: { length_s: 3600, default: false, start: iso(-18), end: iso(42), overdue_since: null, told: 0, left_s: 2520 },
      segments: [
        { kind: 'queued', on: null, text: 'queued', task: null, from: iso(-20), to: iso(-18), seconds: 120 },
        { kind: 'working', on: null, text: '', task: null, from: iso(-18), to: iso(0), seconds: 1080 },
      ],
      memos: [
        { id: 9501, task: 6902, author: ARCH, at: iso(-12), text: 'The plan is ready to read.' },
        { id: 9502, task: 6902, author: ME, at: iso(-9), text: 'Reading it now.' },
      ],
    });
    d.chat.seed(ARCH, {
      from: ARCH,
      to: ME,
      body: 'T6902 is yours. Is it fine to go ahead?', // local-ref-ok
      tasks: [{ id: 6902, title: 'Open task the founder owns' }],
      actions: [
        { key: 'ok', label: 'OK', verb: 'task.done', task: 6902, params: { task: 6902 } },
        { key: 'no', label: 'No', verb: 'task.handoff', task: 6902, params: { task: 6902, to: ARCH }, needs: 'note' },
      ],
    });
  },

  /** The architect is working; one message of the founder is taken as a task, one still waits to be sent, and an answered request sits in the chat. */
  'chat-live': (d) => {
    const at = Date.now();
    const iso = (min: number) => new Date(at + min * 60_000).toISOString();
    d.chat.seed(ARCH, { from: ME, to: ARCH, body: 'Please fix the login screen.', at: iso(-30), request_task: 7140 });
    d.chat.seedItem(ARCH, {
      at: iso(-20),
      kind: 'prompt',
      open: false,
      prompt: {
        id: 'live1',
        agent: ARCH,
        kind: 'permission',
        tool: 'Bash',
        command: 'go vet ./...',
        lines: [],
        options: [],
        since: iso(-20),
      },
    });
    d.chat.seed(ARCH, {
      from: ME,
      to: ARCH,
      body: 'And run the tests after.',
      at: iso(-1),
      landing: 'queued',
      landed_at: null,
      delivered_at: null,
    });
    d.chat.setTurn(ARCH, true);
  },

  /** A long chat with the architect: 120 messages (four pages), every seventh with a picture, for paging up through history. */
  'chat-long': (d) => {
    const png = solidPng(240, 160);
    for (let n = 1; n <= 120; n++) {
      const mine = n % 2 === 0;
      const shot = n % 7 === 0 ? d.uploads.add(`shot-${n}.png`, png) : null;
      d.chat.seed(ARCH, {
        from: mine ? ME : ARCH,
        to: mine ? ARCH : ME,
        body: `Message ${n}. ${'A line of the talk. '.repeat(1 + (n % 5))}`,
        ...(shot ? { attachments: [{ id: shot.id, name: shot.name, mime: 'image/png', bytes: png.length }] } : {}),
      });
    }
  },
  /** Command fences to run in the terminal (T8770): a reply with one never run and one that failed, and the person's own console lines. */
  commands: (d) => {
    const at = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    d.chat.seed(ARCH, {
      from: ME,
      to: ARCH,
      body: 'This is what I ran:\n```console\n$ gh auth status\nYou are not logged in\n```',
      at: at(6),
      landing: 'delivered',
    });
    const id = d.chat.seedItem(ARCH, {
      at: at(5),
      kind: 'reply',
      text: 'Log in needs your password, so run it yourself:\n```bash\ngh auth login --web\n```\nThen check the push again:\n```sh\ngit push origin t8770\n```',
    });
    const item = d.chat.agentReply(ARCH, id)!;
    d.chat.changeItem(ARCH, item, {
      runs: [{ block: `r${id}:1`, terminal: '%91', started_at: at(4), state: 'failed', exit: 128, seconds: 2 }],
    });
  },
};
