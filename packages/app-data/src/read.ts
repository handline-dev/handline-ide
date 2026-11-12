// The read hooks: `useRead` is the one place a read becomes a query, and every named hook is a line over it.
import { useCallback, useMemo, useState } from 'react';
import { keepPreviousData, useQueries, useQuery, type UseQueryResult } from '@tanstack/react-query';
import { EMPTY_DIRECTORY, hashNumbers, nameOf, reviewPictures, waitingCount, type Directory } from '@handline/app-core';
import { useStore } from '@handline/app-react';
import type { CallParams, InboxItem, Methods, Task, WorkspaceRule } from '@handline/client';
import { useData, useFeed, useServes } from '#src/provider.ts';
import { keyOf, qk, reads, type ReadMethod, type ReadSpec } from '#src/reads.ts';

export const SNAPSHOT_STALE_MS = 30_000;

type TimelineEntry = Methods['agent.timeline']['result']['items'][number];

export type ReadResult<M extends ReadMethod> = UseQueryResult<Methods[M]['result']>;

/**
 * A read of the daemon as a query. It runs only while the connection is open and the daemon lists the method; an event-fed read
 * is never stale (the bridge keeps it current) and a snapshot read is stale after 30 s. A list keeps its rows while a filter changes.
 */
export function useRead<M extends ReadMethod>(
  method: M,
  params: CallParams<M>,
  opts: { enabled?: boolean; refetchMs?: number } = {},
): ReadResult<M> {
  const store = useStore();
  const { daemon } = useData();
  const { served, open } = useServes(method);
  const spec: ReadSpec = reads[method];
  useFeed(spec.feed, served);
  return useQuery({
    queryKey: keyOf(daemon, method, params),
    queryFn: () => store.client.call(method, params),
    enabled: served && open && (opts.enabled ?? true),
    staleTime: spec.feed ? Infinity : SNAPSHOT_STALE_MS,
    refetchInterval: opts.refetchMs,
    placeholderData: spec.id ? undefined : keepPreviousData,
  });
}

export type ViewState = 'not_served' | 'loading' | 'error' | 'empty' | 'refreshing' | 'ready';

/** Which of a view's five states to draw (plus *ready*): the skeleton, the error block, the empty sentence, or the data with a quiet marker. */
export function viewState(
  q: Pick<UseQueryResult, 'isPending' | 'isFetching' | 'isError' | 'data'>,
  served: boolean,
  isEmpty = false,
): ViewState {
  if (!served) return 'not_served';
  if (q.isPending) return q.isError ? 'error' : 'loading';
  if (q.isError) return 'error';
  if (isEmpty) return 'empty';
  return q.isFetching ? 'refreshing' : 'ready';
}

export const useAgents = () => useRead('agents.list', {});

/** The three lists every key is written out through (app-core names.ts); a list not read yet is empty, so its keys show as they are. */
export function useDirectory(): Directory {
  const agents = useAgents().data?.agents;
  const people = useRead('people.list', {}).data?.people;
  const workspaces = useRead('workspaces.list', {}).data?.workspaces;
  return useMemo(
    () => ({
      agents: agents ?? EMPTY_DIRECTORY.agents,
      people: people ?? EMPTY_DIRECTORY.people,
      workspaces: workspaces ?? EMPTY_DIRECTORY.workspaces,
    }),
    [agents, people, workspaces],
  );
}

/** nameOf over the directory: what a screen shows for a key. */
export function useNameOf(): (key: string) => string {
  const d = useDirectory();
  return useCallback((key: string) => nameOf(d, key), [d]);
}
/**
 * Messages owed to each agent that have not reached it. The daemon sends `unreached` only for agents.list `status: true`, which is another
 * key from the event-fed list `useAgents` reads, so the bridge does not keep it current: it is read on its own, again every 20 s.
 */
export function useUnreached(): Map<string, number> {
  const q = useRead('agents.list', { status: true }, { refetchMs: 20_000 });
  return useMemo(
    () => new Map((q.data?.agents ?? []).flatMap((a) => (typeof a.unreached === 'number' ? [[a.key, a.unreached] as const] : []))),
    [q.data],
  );
}
export const useAgentDetail = (agent: string) => useRead('agent.get', { agent });
export const useAgentTimeline = (agent: string) => useRead('agent.timeline', { agent });

/**
 * An agent's timeline, newest first: the first page the agents feed keeps current, plus *Older* pages fetched with `before`. Older pages hold the
 * ids below `base`; once new events move the first page past `base` they would leave a gap, so they are dropped.
 */
export function useTimeline(agent: string) {
  const first = useAgentTimeline(agent);
  const store = useStore();
  const [paged, setPaged] = useState<{ agent: string; base: number; items: TimelineEntry[]; more: boolean } | null>(null);
  const [state, setState] = useState<{ busy: boolean; error: unknown }>({ busy: false, error: null });
  const top = first.data ? Math.min(...first.data.items.map((e) => e.id)) : 0;
  const older = paged?.agent === agent && (!first.data?.more || top <= paged.base) ? paged : null;
  const items = useMemo(() => {
    const seen = new Set<number>();
    return [...(first.data?.items ?? []), ...(older?.items ?? [])]
      .filter((e) => !seen.has(e.id) && seen.add(e.id))
      .sort((a, b) => b.id - a.id);
  }, [first.data, older]);
  const more = older ? older.more : (first.data?.more ?? false);
  const loadOlder = useCallback(async () => {
    const before = items.at(-1)?.id;
    if (before === undefined) return;
    setState({ busy: true, error: null });
    try {
      const r = await store.client.call('agent.timeline', { agent, before });
      setPaged({ agent, base: older?.base ?? before, items: [...(older?.items ?? []), ...r.items], more: r.more });
      setState({ busy: false, error: null });
    } catch (error) {
      setState({ busy: false, error });
    }
  }, [store, agent, items, older]);
  return { query: first, items, more, loadOlder, loadingOlder: state.busy, olderError: state.error };
}
/** Which `#N` of a text are pull requests of the agent's repo, and where that repo is: a text with none asks nothing, a daemon without pr.exists or an agent without a GitHub repo finds none. */
export function usePullRequests(agent: string | undefined, text: string): { repo: string | null; isPr: (n: number) => boolean } {
  const numbers = agent ? hashNumbers(text).sort((a, b) => a - b) : [];
  const { data } = useRead('pr.exists', { agent: agent ?? '', numbers }, { enabled: numbers.length > 0 });
  return useMemo(() => {
    const found = new Set(data?.found ?? []);
    return { repo: data?.repo ?? null, isPr: (n: number) => found.has(n) };
  }, [data]);
}
export const useAgentOptions = () => useRead('agent.options', {});
export const useTriggers = (agent?: string) => useRead('agent.triggers', { agent });
export const usePrompts = (agent: string) => useRead('prompt.list', { agent });
export const useStatus = () => useRead('daemon.status', {});
export const useAccounts = () => useRead('accounts.list', {});
/** The accounts with what `hl account list` adds (plan, windows, can it take work and why): a read of its own, fetched again every 30 s because the accounts topic does not carry those fields. */
export const useAccountUsage = () => useRead('accounts.list', { usage: true, roster: true }, { refetchMs: SNAPSHOT_STALE_MS });
export const useInbox = (filter: CallParams<'inbox.list'> = {}) => useRead('inbox.list', filter);
/** The inbox page the feed keeps current plus the older pages fetched with `before` as the end of the list comes near; an item of an older page is not touched by the feed. */
export function useInboxPages() {
  const first = useInbox();
  const store = useStore();
  const [older, setOlder] = useState<{ items: InboxItem[]; more: boolean } | null>(null);
  const [busy, setBusy] = useState(false);
  const head = first.data?.items;
  const items = useMemo(() => {
    const seen = new Set<number>();
    return [...(head ?? []), ...(older?.items ?? [])].filter((i) => !seen.has(i.id) && seen.add(i.id)).sort((a, b) => b.id - a.id);
  }, [head, older]);
  const more = older ? older.more : (first.data?.more ?? false);
  const loadOlder = useCallback(async () => {
    const before = items.at(-1)?.id;
    if (before === undefined || busy) return;
    setBusy(true);
    try {
      const r = await store.client.call('inbox.list', { before });
      setOlder((o) => ({ items: [...(o?.items ?? []), ...r.items], more: r.more }));
    } finally {
      setBusy(false);
    }
  }, [store, items, busy]);
  return { query: first, items, more, loadOlder, loadingOlder: busy };
}
/** The unread items of the whole inbox: the bell, the tab title and the unread dot of the page all read the one cached list, so a mark drops them together. */
export function useUnread(): InboxItem[] {
  const items = useInbox().data?.items;
  return useMemo(() => (items ?? []).filter((i) => !i.read), [items]);
}
export const useInboxItem = (id: number) => useRead('inbox.get', { id });
/** The tasks an agent owns that are not done, read only when asked for (a row's list is opened). */
export const useOpenTasks = (agent: string, enabled: boolean) => useRead('tasks.list', { owner: agent }, { enabled });
/** The founder's requests, newest first, kept current by the requests topic; `state` keeps one status. */
export const useRequests = (state?: CallParams<'requests.list'>['state']) => useRead('requests.list', state ? { state } : {});
export const useTasks = (filter: CallParams<'tasks.list'> = {}, opts: { enabled?: boolean } = {}) => useRead('tasks.list', filter, opts);
/** Every open task (queued or doing) in one read: no limit, no cursor, kept current by the feed. */
export const useOpenTaskList = () => useTasks({ state: 'open' });
export const useDraftTasks = () => useTasks({ state: 'draft' });

/** The closed tasks (done and discarded), newest close first: the first page, and the next ones read by cursor when asked and appended below. */
export function useClosedTasks(enabled = true) {
  const filter = { state: 'closed', order: 'closed', limit: CLOSED_PAGE } as const;
  const first = useRead('tasks.list', filter, { enabled });
  const store = useStore();
  const [older, setOlder] = useState<{ items: Task[]; next: string | null } | null>(null);
  const [busy, setBusy] = useState(false);
  const head = first.data?.items;
  const items = useMemo(() => {
    const seen = new Set<number>();
    return [...(head ?? []), ...(older?.items ?? [])].filter((t) => !seen.has(t.id) && seen.add(t.id));
  }, [head, older]);
  const next = older ? older.next : (first.data?.next ?? null);
  const loadOlder = useCallback(async () => {
    if (!next || busy) return;
    setBusy(true);
    try {
      const r = await store.client.call('tasks.list', { ...filter, cursor: next });
      setOlder((o) => ({ items: [...(o?.items ?? []), ...r.items], next: r.next ?? null }));
    } finally {
      setBusy(false);
    }
  }, [store, next, busy]); // eslint-disable-line react-hooks/exhaustive-deps
  return { query: first, items, more: next !== null, loadOlder, loadingOlder: busy };
}
/** How many closed tasks one page of the Closed list holds. */
export const CLOSED_PAGE = 50;

/** What waits for the founder, by task: the rows that need an answer, the rows with only new comments, and the counts the badges show. */
export const useRequestsPending = () => useRead('requests.pending', {});

/** How many tasks a list shows at first and each time "Show older" is pressed. */
export const TASK_PAGE = 100;

/** The newest page of a task list, kept current by the feed, and the older pages read with `before` when asked; an older row is not touched by the feed. */
export function usePagedTasks(filter: CallParams<'tasks.list'>) {
  const first = useTasks({ ...filter, limit: TASK_PAGE });
  const store = useStore();
  const key = JSON.stringify(filter);
  const [older, setOlder] = useState<{ key: string; items: Task[]; more: boolean } | null>(null);
  const [busy, setBusy] = useState(false);
  const mine = older?.key === key ? older : null; // a changed filter starts over
  const head = first.data?.items;
  const items = useMemo(() => {
    const seen = new Set<number>();
    return [...(head ?? []), ...(mine?.items ?? [])].filter((t) => !seen.has(t.id) && seen.add(t.id)).sort((a, b) => b.id - a.id);
  }, [head, mine]);
  const more = mine ? mine.more : (first.data?.more ?? false);
  const loadOlder = useCallback(async () => {
    const before = items.at(-1)?.id;
    if (before === undefined || busy) return;
    setBusy(true);
    try {
      const r = await store.client.call('tasks.list', { ...filter, limit: TASK_PAGE, before });
      setOlder((o) => ({ key, items: [...(o?.key === key ? o.items : []), ...r.items], more: r.more }));
    } finally {
      setBusy(false);
    }
  }, [store, items, busy, filter, key]);
  return { query: first, items, more, loadOlder, loadingOlder: busy };
}
/** What an agent holds and what it opened: two paged reads (the daemon pages tasks.list and filters by owner or by opener), as one list. */
export function useAgentTasks(agent: string) {
  const held = usePagedTasks({ all: true, owner: agent });
  const opened = usePagedTasks({ all: true, from: agent });
  const query = {
    isPending: held.query.isPending || opened.query.isPending,
    isFetching: held.query.isFetching || opened.query.isFetching,
    isError: held.query.isError || opened.query.isError,
    data: held.query.data && opened.query.data ? [held.query.data, opened.query.data] : undefined,
    error: held.query.error ?? opened.query.error,
    refetch: () => Promise.all([held.query.refetch(), opened.query.refetch()]),
  };
  const items = useMemo(() => [...held.items, ...opened.items], [held.items, opened.items]);
  const more = held.more || opened.more;
  const loadOlder = useCallback(async () => {
    await Promise.all([held.more ? held.loadOlder() : null, opened.more ? opened.loadOlder() : null]);
  }, [held, opened]);
  return { query, items, more, loadOlder, loadingOlder: held.loadingOlder || opened.loadingOlder };
}
export const useAgentStrip = (agent: string) => useRead('agent.strip', { agent }, { enabled: agent !== '' });
export const useTask = (task: number, opts: { enabled?: boolean } = {}) => useRead('task.get', { task }, opts);
/** The titles of these tasks, each read by its id (the same read, and cache, as `useTask`): for a task the list of the newest tasks no longer holds. */
export const useTaskTitles = (ids: readonly number[]): Map<number, string> => useTaskTitleReads(ids).titles;
/** The same, and whether any of the reads is still on its way. */
export function useTaskTitleReads(ids: readonly number[]): { titles: Map<number, string>; pending: boolean } {
  const store = useStore();
  const { daemon } = useData();
  const { served, open } = useServes('task.get');
  const results = useQueries({
    queries: ids.map((task) => ({
      queryKey: keyOf(daemon, 'task.get', { task }),
      queryFn: () => store.client.call('task.get', { task }),
      enabled: served && open,
      staleTime: SNAPSHOT_STALE_MS,
    })),
  });
  return {
    titles: new Map(results.flatMap((r) => (r.data ? [[r.data.id, r.data.title] as const] : []))),
    pending: results.some((r) => r.isPending && r.fetchStatus !== 'idle'),
  };
}
export const useTaskHistory = (task: number) => useRead('task.history', { task });
export const useTaskTime = (task: number) => useRead('task.time', { task });
export const useMemos = (task: number) => useRead('task.comment.list', { task });
/** A review's pictures; empty while the comments load or when the first comment has none. */
export const useReviewPictures = (task: number) => reviewPictures(useMemos(task).data?.memos ?? []);
export const useTaskBoard = () => useRead('task.board', {});
export const useTrust = () => useRead('trust.list', {});
/** Logins already on this Mac outside Handline (~/.claude, ~/.codex), for onboarding's AI step: read when the screen opens, not kept. */
export function useFoundLogins() {
  const store = useStore();
  const { daemon } = useData();
  const { served, open } = useServes('accounts.discover');
  return useQuery({
    queryKey: [daemon, 'accounts.discover'],
    queryFn: () => store.client.call('accounts.discover', {}),
    enabled: served && open,
    gcTime: 0,
    select: (r) => r.found,
  });
}
/** Each kept rule's place in the rules column; a Record, so a key the contract adds fails to compile until it has one. */
const RULE_ORDER: Record<WorkspaceRule['key'], number> = {
  accounts: 0,
  agent_cap: 1,
  mode_max: 2,
  tools_deny: 3,
  review_required: 4,
  'lease_max.heavy': 5,
  'lease_max.simulator': 6,
  'lease_max.browser': 7,
  'lease_max.port': 8,
  land_by: 9,
  land_check: 10,
};
/** Every kept rule's key, in the order the rules column lists them. */
export const RULE_KEYS = (Object.keys(RULE_ORDER) as WorkspaceRule['key'][]).sort((a, b) => RULE_ORDER[a] - RULE_ORDER[b]);

/** A kept rule as the rules column shows it: one never set on the workspace or the machine has no value and no source. */
export type RuleRow = { key: WorkspaceRule['key'] } & (
  (Omit<WorkspaceRule, 'key'> & { set: true }) | { set: false; on: false; value: null; source: null }
);

/** Each key's row from the rules in force: the daemon lists only the keys set somewhere. */
export function ruleRows(rules: readonly WorkspaceRule[]): RuleRow[] {
  const byKey = new Map(rules.map((r) => [r.key, r]));
  return RULE_KEYS.map((key) => {
    const r = byKey.get(key);
    return r ? { ...r, set: true } : { key, set: false, on: false, value: null, source: null };
  });
}

/** A workspace's kept rules, every key (ruleRows), and its rules text's size and limit in bytes; without a workspace the machine
 * default's rules and no size. No topic feeds them: read when the screen opens and again after workspace.rule.set. */
export function useWorkspaceRules(workspace?: string) {
  const store = useStore();
  const { daemon } = useData();
  const { served, open } = useServes('workspace.rules');
  return useQuery({
    queryKey: qk.rules(daemon, workspace ?? null),
    queryFn: () => store.client.call('workspace.rules', workspace === undefined ? {} : { workspace }),
    enabled: served && open,
    select: (r) => ({ rules: ruleRows(r.rules), textBytes: r.text_bytes ?? null, limitBytes: r.limit_bytes ?? null }),
  });
}
/** The newest events of the daemon's log, read once when the page opens (the log is not a cache the bridge keeps; a screen that wants it live asks again). */
export function useRecentEvents(limit: number) {
  const store = useStore();
  const { daemon } = useData();
  const { served, open } = useServes('events.feed');
  return useQuery({
    queryKey: [daemon, 'events.feed', limit],
    queryFn: () => store.client.call('events.feed', { limit }),
    enabled: served && open,
    gcTime: 0,
    select: (r) => [...r.events].sort((a, b) => b.cursor - a.cursor),
  });
}
export const useMemory = (agent?: string) => useRead('memory.status', { agent });
export const useTeam = () => useRead('team.get', {});
export const useProposals = (filter: CallParams<'proposal.list'> = {}) => useRead('proposal.list', filter);
/** How many proposals wait on the founder; 0 while the list is not here or not served. */
export const useProposalCount = (): number => waitingCount(useProposals().data?.proposals ?? []);
export const usePrefs = () => useRead('prefs.get', {});
export const useRepos = () => useRead('repos.list', {});
export const useWorktrees = (filter: CallParams<'worktrees.list'> = {}) => useRead('worktrees.list', filter);
export const useFile = (agent: string, path: string) => useRead('file.read', { agent, path }, { enabled: path !== '' });
export const useSchedules = () => useRead('schedules.list', {});
export const useStats = (window: CallParams<'stats.get'>['window']) => useRead('stats.get', { window });
export const useOutlook = (range: CallParams<'stats.outlook'>['range']) => useRead('stats.outlook', { range });
export const useSettings = (agent?: string) => useRead('settings.get', { agent });
export const useConnection = () => useRead('connection.get', {});
export const useAnswers = () => useRead('answers.list', {});
export const useDevices = () => useRead('devices.list', {});
export const useOperation = (operation: string) => useRead('operation.get', { operation });
/** Where a message number sits (its chat and scope); `enabled` false reads nothing until a message number is clicked. */
export const useChatLocate = (params: CallParams<'chat.locate'>, enabled = true) => useRead('chat.locate', params, { enabled });
export const useChatRefs = (agent: string, enabled = true) => useRead('chat.refs', { agent }, { enabled });
export const useChatSearch = (params: CallParams<'chat.search'>, enabled = true) => useRead('chat.search', params, { enabled });

/** A one-off load that is not a daemon read (the desktop shell's answer, a picture's bytes) as a query: kept per `key`, undefined until it lands or when `enabled` is off. */
export function useLoaded<T>(key: readonly unknown[], load: () => Promise<T>, enabled = true): T | undefined {
  return useQuery({ queryKey: ['loaded', ...key], queryFn: load, enabled, staleTime: Infinity }).data;
}
