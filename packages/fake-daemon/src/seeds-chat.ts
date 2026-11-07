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
};
