import type { Account } from '@handline/client';
import { Segmented } from '#src/ui/Segmented.tsx';
import { AccountFacts } from '#src/usage/AccountFacts.tsx';
import { RowsLoading } from '#src/usage/UsageViews.tsx';
import { useAccountUsage, useAccounts, useWrite } from '@handline/app-data';
import { RefreshCw, Settings, TriangleAlert } from 'lucide-react';
import { focusAfter } from '@handline/app-core';
import { useCallback, useEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react';
import { InCluster, useCluster, useJoined } from '#src/connection/clusters.tsx';
import { HereWord } from '#src/connection/ClusterWord.tsx';
import { useFocused } from '#src/layout/focus.ts';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { openSettings } from '#src/settings/open.ts';
import { AccountActions } from '#src/usage/AccountsTab.tsx';
import { renameAccount } from '#src/usage/AccountParts.tsx';
import { toast } from '#src/ui/toast.tsx';
import { uuid } from '#src/ui/uuid.ts';
import { Outlook } from '#src/usage/Outlook.tsx';
import {
  ByAgent,
  ByModel,
  COST_RANGES,
  CostPerDay,
  VIEWS,
  VIEW_ICON,
  VIEW_WORD,
  WINDOWS,
  type CostRange,
  type View,
  type Win,
} from '#src/usage/UsageViews.tsx';
import { usePersisted } from '#src/persist/kv.ts';

/** The week left of all the accounts as a graph, then every account on one row: its windows, the agents on it, its login. An account asked for from
 * elsewhere (an agent's account line) comes into view, marked for a moment. */
export function Usage() {
  const list = useAccounts();
  const joined = useJoined();
  const [view, setView] = usePersisted<View>('usage:view', 'accounts');
  const [win, setWin] = usePersisted<Win>('usage:window', '24h');
  const [range, setRange] = usePersisted<CostRange>('usage:cost-range', '7d');
  // The account the graph is focused on; null: all of them.
  const [focus, setFocus] = usePersisted<string | null>('usage:focus', null);
  const accounts = list.data?.accounts ?? [];
  const box = useRef<HTMLDivElement>(null);
  const [marked, setMarked] = useState<string | null>(null);
  const show = useCallback((name: string) => {
    box.current?.querySelector(`[data-account="${CSS.escape(name)}"]`)?.scrollIntoView({ block: 'start' });
    setMarked(name);
    setTimeout(() => setMarked(null), 1500);
  }, []);
  useFocused('usage', '', !!list.data, show);
  return (
    <ScrollFade keep="usage" className="min-h-0 flex-1">
      <Page>
        <div ref={box} className="flex flex-col gap-3 py-4">
          <Outlook labels={Object.fromEntries(accounts.map((a) => [a.name, a.label]))} focus={focus} onAll={() => setFocus(null)} />
          {/* Under the graph, one view at a time. */}
          {/* The views' bar stays at the top while their list scrolls; the view's own range sits at its right. */}
          <div className="@container sticky top-0 z-10 -mx-4 mt-4 flex flex-wrap items-center justify-between gap-3 bg-bg px-4 py-2">
            <Segmented label="View" value={view} options={VIEWS} word={(v) => VIEW_WORD[v]} icon={(v) => VIEW_ICON[v]} onChange={setView} />
            {(view === 'agents' || view === 'models') && (
              <Segmented label="Window" value={win} options={WINDOWS} word={(w) => w} onChange={setWin} />
            )}
            {view === 'cost' && <Segmented label="Range" value={range} options={COST_RANGES} word={(r) => r} onChange={setRange} />}
          </div>
          {view === 'agents' && <ByAgent win={win} />}
          {view === 'models' && <ByModel win={win} />}
          {view === 'cost' && <CostPerDay range={range} />}
          {/* Each account on one row, to read them all at once: its name, its two windows, and the agents on it. */}
          {view === 'accounts' && list.isPending && <RowsLoading rows={6} />}
          {view === 'accounts' && list.isError && <p className="text-sm text-danger">Accounts did not load.</p>}
          <div className={`@container flex flex-col ${view === 'accounts' ? '' : 'hidden'}`}>
            <AccountRows marked={marked} focus={focus} onFocus={(n) => setFocus(focusAfter(focus, n))} />
            {joined.map((j) => (
              <InCluster key={j.id} id={j.id} dim>
                <AccountRows marked={null} word={<HereWord />} />
              </InCluster>
            ))}
          </div>
          {/* One way to manage them, at the end of the list. */}
          {view === 'accounts' && (
            <Button variant="ghost" className="mx-auto" onClick={() => openSettings({ section: 'accounts' })}>
              Manage accounts →
            </Button>
          )}
        </div>
      </Page>
    </ScrollFade>
  );
}

/** Each account on one row, to read them all at once: its name, its two windows, and the agents on it; another cluster's with its name. */
function AccountRows({
  marked,
  word,
  focus,
  onFocus,
}: {
  marked: string | null;
  word?: ReactNode;
  /** This cluster's rows only: a click on a row focuses the graph on it. */
  focus?: string | null;
  onFocus?: (name: string) => void;
}) {
  const cluster = useCluster();
  const accounts = useAccounts().data?.accounts ?? [];
  // The plan (Ultra, Max, Pro) comes only with the usage read.
  const usage = useAccountUsage().data?.accounts ?? [];
  const plans = new Map(usage.map((u) => [u.name, u.plan]));
  // Why the daemon would not pick an account now (its week cap, an ended subscription); absent when it would.
  const notPicked = new Map(usage.flatMap((u) => (u.not_picked ? [[u.name, u.not_picked] as const] : [])));
  // The account whose settings are open under its row, as Settings > Accounts opens a row.
  const [open, setOpen] = useState<string | null>(null);
  return (
    <>
      {accounts.map((a) => (
        <div key={a.name} className="border-b border-border">
          <section
            data-account={a.name}
            {...(onFocus && {
              role: 'button',
              tabIndex: 0,
              'aria-pressed': focus === a.name,
              'aria-label': `${focus === a.name ? 'Show all accounts in the graph' : `Show ${a.label} in the graph`}`,
              // A button inside the row (rename, read again, settings, an agent) keeps its own click.
              onClick: (e: MouseEvent) => !(e.target as Element).closest('button,a') && onFocus(a.name),
              onKeyDown: (e: KeyboardEvent) =>
                e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), onFocus(a.name)),
            })}
            className={`relative flex scroll-mt-4 flex-col gap-2 @md:flex-row @md:items-center @md:gap-x-6 py-3 transition-colors ${onFocus ? 'cursor-pointer' : ''} ${marked === a.name || focus === a.name ? 'bg-selected' : ''}`}
          >
            <div className="@container min-w-0 flex-1 pr-16 @md:pr-0">
              <AccountFacts
                account={a}
                onRename={() => void renameAccount(a)}
                plan={plans.get(a.name)}
                note={<NotPicked why={notPicked.get(a.name)} needsLogin={a.login_state === 'needs_login'} />}
              />
            </div>
            <div className="absolute top-3 right-0 flex @md:static">
              {word}
              <ReadAgain account={a} />
              <IconButton
                label={`Settings for ${a.label}`}
                aria-expanded={cluster ? undefined : open === a.name}
                onClick={() =>
                  cluster ? openSettings({ section: 'accounts', account: a.name, cluster }) : setOpen(open === a.name ? null : a.name)
                }
              >
                <Settings size={14} />
              </IconButton>
            </div>
          </section>
          {open === a.name && (
            <div className="pb-3">
              <AccountActions account={a} out={a.login_state === 'needs_login'} />
            </div>
          )}
        </div>
      ))}
    </>
  );
}

/** Why new agents do not go to this account, only when the founder must fix it (an ended subscription, a login): a full window
    already shows on its bars. */
function NotPicked({ why, needsLogin }: { why: Account['not_picked'] | undefined; needsLogin: boolean }) {
  if (!why || (why.kind === 'login' && needsLogin)) return null; // Needs login already says so
  if (why.kind !== 'subscription_ended' && why.kind !== 'login') return null;
  return (
    <span className="flex items-start gap-1 text-xs text-danger" title={why.text}>
      <TriangleAlert size={12} className="mt-0.5 shrink-0" />
      <span className="line-clamp-2">Not used: {why.text}</span>
    </span>
  );
}

/** Reads one account's usage again now. The daemon reads it at its next poll tick, so the icon spins until a reading younger
 * than the one shown at the click arrives (or a minute passes). */
function ReadAgain({ account: a }: { account: Account }) {
  const read = useWrite('account.read_usage', a.name);
  // The reading's age at the click (Infinity: none yet); null when not asked.
  const [asked, setAsked] = useState<number | null>(null);
  // A younger reading came: done asking (set during render, as React's "adjusting state on a prop change").
  if (asked != null && a.age_s != null && a.age_s < asked) setAsked(null);
  useEffect(() => {
    if (asked == null) return;
    const t = setTimeout(() => setAsked(null), 60_000);
    return () => clearTimeout(t);
  }, [asked]);
  const busy = asked != null || read.isPending;
  return (
    <IconButton
      label={`Read ${a.label}'s usage again`}
      disabled={busy || a.login_state === 'needs_login'}
      onClick={() => {
        setAsked(a.age_s ?? Infinity);
        read.mutate(
          { account: a.name, request_id: uuid() },
          {
            onError: (e) => {
              setAsked(null);
              toast(`Usage was not read again: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });
            },
          },
        );
      }}
    >
      <RefreshCw size={14} className={busy ? 'animate-spin' : ''} />
    </IconButton>
  );
}
