import type { Account } from '@handline/client';
import { RowsLoading } from '#src/usage/UsageViews.tsx';
import { useAccountUsage, useAccounts } from '@handline/app-data';
import { TriangleAlert } from 'lucide-react';
import { useCallback, useRef, useState } from 'react';
import { useFocused } from '#src/layout/focus.ts';
import { goPage } from '#src/layout/openTab.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { Outlook } from '#src/usage/Outlook.tsx';
import {
  ByAgent,
  ByModel,
  COST_RANGES,
  CostPerDay,
  Switch,
  VIEWS,
  VIEW_ICON,
  VIEW_WORD,
  WINDOWS,
  type CostRange,
  type View,
  type Win,
} from '#src/usage/UsageViews.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { FIVE_HOURS, UsageBar, WEEK } from '#src/usage/UsageBar.tsx';

/** The week left of all the accounts as a graph, then every account on one row: its windows, the agents on it, its login. An account asked for from
 * elsewhere (an agent's account line) comes into view, marked for a moment. */
export function Usage() {
  const list = useAccounts();
  // The plan (max, pro) comes only with the usage read.
  const usage = useAccountUsage().data?.accounts ?? [];
  const plans = new Map(usage.map((u) => [u.name, u.plan]));
  // Why the daemon would not pick an account now (its week cap, an ended subscription); absent when it would.
  const notPicked = new Map(usage.flatMap((u) => (u.not_picked ? [[u.name, u.not_picked] as const] : [])));
  const [view, setView] = usePersisted<View>('usage:view', 'accounts');
  const [win, setWin] = usePersisted<Win>('usage:window', '24h');
  const [range, setRange] = usePersisted<CostRange>('usage:cost-range', '7d');
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
          <Outlook labels={Object.fromEntries(accounts.map((a) => [a.name, a.label]))} />
          {/* Under the graph, one view at a time. */}
          {/* The views' bar stays at the top while their list scrolls; the view's own range sits at its right. */}
          <div className="sticky top-0 z-10 -mx-4 mt-4 flex items-center justify-between gap-3 bg-bg px-4 py-2">
            <Switch label="View" value={view} options={VIEWS} word={(v) => VIEW_WORD[v]} icon={(v) => VIEW_ICON[v]} onChange={setView} />
            {(view === 'agents' || view === 'models') && (
              <Switch label="Window" value={win} options={WINDOWS} word={(w) => w} onChange={setWin} />
            )}
            {view === 'cost' && <Switch label="Range" value={range} options={COST_RANGES} word={(r) => r} onChange={setRange} />}
          </div>
          {view === 'agents' && <ByAgent win={win} />}
          {view === 'models' && <ByModel win={win} />}
          {view === 'cost' && <CostPerDay range={range} />}
          {/* Each account on one row, to read them all at once: its name, its two windows, and the agents on it. */}
          {view === 'accounts' && list.isPending && <RowsLoading rows={6} />}
          {view === 'accounts' && list.isError && <p className="text-sm text-danger">Accounts did not load.</p>}
          <div className={`flex flex-col ${view === 'accounts' ? '' : 'hidden'}`}>
            {accounts.map((a) => (
              <section
                key={a.name}
                data-account={a.name}
                className={`flex scroll-mt-4 flex-wrap items-center gap-x-6 gap-y-2 border-b border-border py-3 transition-colors ${marked === a.name ? 'bg-selected' : ''}`}
              >
                <div className="flex w-36 min-w-0 flex-col">
                  <span className="flex items-baseline gap-1.5 truncate text-sm font-medium text-fg">
                    {a.label}
                    {plans.get(a.name) && <span className="text-xs font-normal text-muted capitalize">{plans.get(a.name)}</span>}
                  </span>
                  {a.login_state === 'needs_login' ? (
                    <span className="flex items-center gap-1 text-xs text-danger">
                      <TriangleAlert size={12} />
                      Needs login
                    </span>
                  ) : (
                    <span className="truncate text-xs text-muted" title={a.agents.join(', ')}>
                      {a.agents.length ? `${a.agents.length} agents · up to ${a.cap_effective}` : 'No agents'}
                    </span>
                  )}
                  <NotPicked why={notPicked.get(a.name)} needsLogin={a.login_state === 'needs_login'} />
                </div>
                {/* An account that needs login cannot be used: its windows read dimmed. */}
                <div className={`min-w-40 flex-1 ${a.login_state === 'needs_login' ? 'opacity-40' : ''}`}>
                  <UsageBar name="5-hour" span={FIVE_HOURS} w={a.five_hour} />
                </div>
                <div className={`min-w-40 flex-1 ${a.login_state === 'needs_login' ? 'opacity-40' : ''}`}>
                  <UsageBar name="Week" span={WEEK} w={a.weekly} />
                </div>
              </section>
            ))}
          </div>
          {/* One way to manage them, at the end of the list. */}
          {view === 'accounts' && (
            <Button variant="ghost" className="mx-auto" onClick={() => goPage('preferences', 'Settings')}>
              Manage accounts →
            </Button>
          )}
        </div>
      </Page>
    </ScrollFade>
  );
}

/** Why new agents do not go to this account: loud when only the founder can fix it (an ended subscription, a login). */
function NotPicked({ why, needsLogin }: { why: Account['not_picked'] | undefined; needsLogin: boolean }) {
  if (!why || (why.kind === 'login' && needsLogin)) return null; // Needs login already says so
  const loud = why.kind === 'subscription_ended' || why.kind === 'login';
  return (
    <span className={`flex items-start gap-1 text-xs ${loud ? 'text-danger' : 'text-muted'}`} title={why.text}>
      {loud && <TriangleAlert size={12} className="mt-0.5 shrink-0" />}
      <span className="line-clamp-2">Not used: {why.text}</span>
    </span>
  );
}
