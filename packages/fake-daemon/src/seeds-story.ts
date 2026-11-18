// A story for the Story screen: a goal the founder worked out with the lead in its comments, and five drafts under it, each waiting
// on the one it starts after, their plans in every state a node shows. story-review is a story whose sub-tasks were run: one finished
// and Ready for the founder, one sent back once and being worked again.
import type { FakeDaemon } from '#src/daemon.ts';
import type { PlanVersion } from '#src/planview.ts';
import { AFTER_JPG, BEFORE_JPG } from '#src/seedpictures.ts';
import { DEMO_MP4 } from '#src/seedvideo.ts';

type Seed = (d: FakeDaemon) => void;

const ARCH = 'handline.architect'; // local-ref-ok
const AUD = 'handline.auditor'; // local-ref-ok
const TERM = 'handline.term-build'; // local-ref-ok
const ME = 'human';

type Draft = {
  id: number;
  to: string;
  title: string;
  after: number | null;
  plan?: { versions: PlanVersion[]; ok: { by: string; at: string } | null };
  writing?: boolean;
};

/** The five drafts, each after the one it starts after. */
function storyDrafts(iso: (min: number) => string): Draft[] {
  const v = (by: string, min: number, text: string): PlanVersion => ({ by, at: iso(min), text });
  return [
    {
      id: 9401,
      to: TERM,
      title: 'Index chat messages for search',
      after: null,
      plan: {
        versions: [
          v(
            TERM,
            60,
            'Change: a full-text index of chat messages, kept as they arrive.\nFiles: go/data/db/chatindex.go\nDone: a test finds a word in a message written a minute ago.\nLeft out: other workspaces.',
          ),
        ],
        ok: { by: ME, at: iso(40) },
      },
    },
    {
      id: 9402,
      to: AUD,
      title: 'chat.search on the index',
      after: 9401,
      plan: {
        versions: [
          v(
            AUD,
            58,
            'Change: chat.search reads the index.\nFiles: go/internal/reads/chatsearch.go\nDone: a fixture finds a message by a word.',
          ),
          v(
            AUD,
            20,
            'Change: chat.search reads the index, newest first, 50 at a time.\nFiles: go/internal/reads/chatsearch.go\nDone: a fixture finds a message by a word.',
          ),
        ],
        ok: null,
      },
    },
    {
      id: 9403,
      to: ARCH,
      title: 'Search box on the chat screen',
      after: 9402,
      plan: {
        versions: [
          v(
            ARCH,
            30,
            'Change: a search box above the chat; Enter jumps to the newest match.\nFiles: packages/app-react/src/chat/\nDone: before and after pictures.\nStarts after: chat.search.',
          ),
        ],
        ok: null,
      },
    },
    { id: 9404, to: TERM, title: 'Highlight the matches in a message', after: 9403, writing: true },
    {
      id: 9405,
      to: AUD,
      title: 'Search across workspaces',
      after: 9402,
      plan: {
        versions: [
          v(
            AUD,
            15,
            'Change: chat.search takes every workspace the founder sees.\nFiles: go/internal/reads/chatsearch.go\nQuestion: should a member device see other workspaces?',
          ),
        ],
        ok: null,
      },
    },
  ];
}

export const STORY_SEEDS: Record<string, Seed> = {
  /** The story and its five drafts: one plan OK, two to read (one in its second version), one being written, one with a question. */
  story: (d) => {
    const iso = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    d.tasks.verbs = true; // Run is task.queue
    const story = 9400;
    const chat = [
      { id: 1, at: iso(90), author: ME, text: 'I want to find an old message in a chat without scrolling.' },
      { id: 2, at: iso(86), author: ARCH, text: 'Search over every chat, or one agent at a time?' },
      { id: 3, at: iso(80), author: ME, text: 'One chat first. Across workspaces later.' },
      {
        id: 4,
        at: iso(72),
        author: ARCH,
        text: 'Five steps: index the messages, a search call, the search box, highlighted matches, then search across workspaces. They are drafts on the right; nothing has been sent.',
      },
    ].map((m) => ({ ...m, task: story }));
    const drafts = storyDrafts(iso);
    d.tasks.seed({
      id: story,
      my_request: true,
      from: ME,
      to: ARCH,
      title: 'Story: search the chat history',
      state: 'doing',
      created: iso(95),
      body: 'Find an old message in a chat without scrolling.',
      memos: chat,
      blockers: drafts.map((t) => ({ task: t.id, reason: '' })),
    });
    // The question in T9405's plan, put to the founder on the task. // local-ref-ok
    const ask = 'Should a member device see the other workspaces in search, or only the one it is paired to?';
    const question = {
      memos: [{ id: 1, task: 9405, at: iso(14), author: AUD, text: ask }],
      turnLog: [{ at: iso(14), who: `${AUD} → ${ME}`, text: ask, turn: 'push' as const, memo: 1 }],
      turns: [{ agent: ME, ask, memo: 1, by: AUD, since: iso(14), on_pass: null, evidence: null, edited: null, earlier: [] }],
      acting: ME,
    };
    for (const t of drafts) {
      d.tasks.seed({
        id: t.id,
        from: ARCH,
        to: t.to,
        title: t.title,
        state: 'draft',
        parent: story,
        created: iso(70),
        body: `${t.title}. Part of T${story}.`,
        blockers: t.after ? [{ task: t.after, reason: '' }] : [],
        ...(t.plan ? { plan: t.plan } : {}),
        ...(t.writing ? { planAsk: { at: iso(10), by: 'sys', to: t.to, sent_at: iso(8) } } : {}),
        ...(t.id === 9405 ? question : {}),
      });
    }
  },

  /** A story with two sub-tasks run: one Ready for you with before and after pictures and a video, one working again after a Redo. */
  'story-review': (d) => {
    const iso = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
    d.tasks.verbs = true;
    const [story, ready, redo] = [9410, 9411, 9412];
    d.tasks.seed({
      id: story,
      my_request: true,
      from: ME,
      to: ARCH,
      title: 'Story: a badge for unread chats',
      state: 'doing',
      created: iso(200),
      body: 'Show how many chats are unread.',
      blockers: [ready, redo].map((task) => ({ task, reason: '' })),
    });
    const sub = (id: number, to: string, title: string, min: number) => ({
      id,
      from: ARCH,
      to,
      title,
      parent: story,
      created: iso(min),
      body: `${title}. Part of T${story}.`,
      blockers: [],
      plan: { versions: [{ by: to, at: iso(min - 10), text: `Change: ${title.toLowerCase()}.` }], ok: { by: ME, at: iso(min - 20) } },
    });
    d.tasks.seed({ ...sub(ready, TERM, 'Count unread chats', 150), state: 'doing' });
    const [before, after, poster] = [
      d.uploads.add('before.jpg', BEFORE_JPG),
      d.uploads.add('after.jpg', AFTER_JPG),
      d.uploads.add('badge.jpg', AFTER_JPG),
    ];
    const video = { ...d.uploads.memoFile(d.uploads.add('badge.mp4', DEMO_MP4, 'video/mp4').id), poster: poster.id };
    const files = [before, after, poster].map((f) => d.uploads.memoFile(f.id)).concat(video);
    const result = 'Done: the count comes from chat.list. Changed from the plan: nothing. Before and after, and the badge counting up.';
    const memo = d.tasks.memoWithFiles(ready, TERM, result, files);
    d.tasks.turnPush(ready, memo, result, TERM);
    d.tasks.find(ready).review = { ready: true, land_by: ARCH, memo, at: iso(5), redo: 0, ok: false };
    // its code: a commit on the agent's branch that names the task
    const file = (path: string, added: number, deleted: number, diff: string) => ({
      path,
      repo: 'handline',
      added,
      deleted,
      binary: false,
      diff,
    });
    d.tasks.find(ready).commits = [
      {
        sha: '9411a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7',
        repo: 'handline',
        subject: `Unread count on each chat (T${ready})`,
        agent: TERM,
        at: iso(40),
        on_main: false,
        files: [
          file(
            'packages/app-data/src/chats.ts',
            1,
            0,
            "diff --git a/packages/app-data/src/chats.ts b/packages/app-data/src/chats.ts\n--- a/packages/app-data/src/chats.ts\n+++ b/packages/app-data/src/chats.ts\n@@ -1 +1,2 @@\n export * from './list';\n+export { unread } from './unread';\n",
          ),
          file(
            'packages/app-data/src/unread.ts',
            1,
            0,
            'diff --git a/packages/app-data/src/unread.ts b/packages/app-data/src/unread.ts\nnew file mode 100644\n--- /dev/null\n+++ b/packages/app-data/src/unread.ts\n@@ -0,0 +1 @@\n+export const unread = (n: number) => Math.max(0, n);\n',
          ),
        ],
      },
    ];

    d.tasks.seed({ ...sub(redo, AUD, 'Draw the badge on the chat list', 140), state: 'doing' });
    const first = d.tasks.memo({ task: redo, text: 'Done: a red badge on each chat row.' }, 'add', AUD, true);
    d.tasks.memo({ task: redo, text: 'Redo: the badge covers the agent name on the phone.' }, 'add', ME);
    d.tasks.find(redo).review = { ready: false, land_by: ARCH, memo: first, at: iso(30), redo: 1, ok: false };
  },
};
