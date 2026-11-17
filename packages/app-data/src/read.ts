// The read hooks: `useRead` is the one place a read becomes a query, and every named hook is a line over it.
import { useCallback, useMemo } from 'react';
import { keepPreviousData, useQuery, type UseQueryResult } from '@tanstack/react-query';
import { EMPTY_DIRECTORY, nameOf, type Directory } from '@handline/app-core';
import { useStore } from '@handline/app-react';
import type { CallParams, InboxItem, Methods, WorkspaceRule } from '@handline/client';
import { useData, useFeed, useServes } from '#src/provider.ts';
import { keyOf, qk, reads, type ReadMethod, type ReadSpec } from '#src/reads.ts';

export const SNAPSHOT_STALE_MS = 30_000;

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

/** nameOf over the directory: what a screen shows for a key; `here`, a workspace key, when the screen is that workspace's. */
export function useNameOf(here?: string): (key: string) => string {
  const d = useDirectory();
  return useCallback((key: string) => nameOf(d, key, here), [d, here]);
}
export const useAgentDetail = (agent: string) => useRead('agent.get', { agent });

export const useAgentOptions = () => useRead('agent.options', {});
export const useStatus = () => useRead('daemon.status', {});
export const useAccounts = () => useRead('accounts.list', {});
/** The accounts with what `hl account list` adds (plan, windows, can it take work and why): a read of its own, fetched again every 30 s because the accounts topic does not carry those fields. */
export const useAccountUsage = () => useRead('accounts.list', { usage: true, roster: true }, { refetchMs: SNAPSHOT_STALE_MS });
export const useInbox = (filter: CallParams<'inbox.list'> = {}) => useRead('inbox.list', filter);
/** The unread items of the whole inbox: the bell, the tab title and the unread dot of the page all read the one cached list, so a mark drops them together. */
export function useUnread(): InboxItem[] {
  const items = useInbox().data?.items;
  return useMemo(() => (items ?? []).filter((i) => !i.read), [items]);
}
export const useTasks = (filter: CallParams<'tasks.list'> = {}, opts: { enabled?: boolean } = {}) => useRead('tasks.list', filter, opts);

export const useAgentStrip = (agent: string) => useRead('agent.strip', { agent }, { enabled: agent !== '' });
export const useTask = (task: number, opts: { enabled?: boolean } = {}) => useRead('task.get', { task }, opts);
export const useTaskHistory = (task: number) => useRead('task.history', { task });
export const useTaskTime = (task: number) => useRead('task.time', { task });
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
export const useProposals = (filter: CallParams<'proposal.list'> = {}) => useRead('proposal.list', filter);
export const usePrefs = () => useRead('prefs.get', {});
export const useRepos = () => useRead('repos.list', {});
export const useWorktrees = (filter: CallParams<'worktrees.list'> = {}) => useRead('worktrees.list', filter);
export const useStats = (window: CallParams<'stats.get'>['window']) => useRead('stats.get', { window });
export const useOutlook = (range: CallParams<'stats.outlook'>['range']) => useRead('stats.outlook', { range });
export const useSettings = (agent?: string) => useRead('settings.get', { agent });
export const useOperation = (operation: string) => useRead('operation.get', { operation });
