// A story for the Story screen: a goal the founder worked out with the lead in its comments, and five drafts under it, each waiting
// on the one it starts after, their plans in every state a node shows.
import type { FakeDaemon } from '#src/daemon.ts';
import type { PlanVersion } from '#src/planview.ts';

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
            'Change: chat.search reads the index, newest first, 50 at a time.\nFiles: go/internal/reads/chatsearch.go, contract/spec/chat.json\nDone: a fixture finds a message by a word, and pages.\nStarts after: the index.',
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
};
