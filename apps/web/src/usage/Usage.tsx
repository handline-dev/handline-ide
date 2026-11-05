import { useAccounts } from '@handline/app-data';
import { TriangleAlert } from 'lucide-react';
import { useCallback, useRef, useState } from 'react';
import { useFocused } from '#src/layout/focus.ts';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { FIVE_HOURS, UsageBar, WEEK } from '#src/usage/UsageBar.tsx';

/** Every account: its windows, the agents on it against its cap, its login, and its subscription's end. An account asked for from
 * elsewhere (an agent's account line) comes into view, marked for a moment. */
export function Usage() {
  const list = useAccounts();
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
          <h1 className="text-lg font-semibold text-fg">Usage</h1>
          {list.isPending && <p className="text-sm text-muted">Loading…</p>}
          {list.isError && <p className="text-sm text-danger">Accounts did not load.</p>}
          {accounts.map((a) => (
            <section
              key={a.name}
              data-account={a.name}
              className={`flex scroll-mt-4 flex-col gap-3 rounded-md border p-4 transition-colors ${marked === a.name ? 'border-accent' : 'border-border'}`}
            >
              <div className="flex items-baseline gap-2">
                <span className="text-base font-medium text-fg">{a.label}</span>
                {a.label !== a.name && <span className="text-xs text-muted">{a.name}</span>}
                {a.login_state === 'needs_login' && (
                  <span className="flex items-center gap-1 text-xs text-danger">
                    <TriangleAlert size={12} />
                    Needs login
                  </span>
                )}
                <span className="flex-1" />
                {a.cycle.on && a.cycle.end && <span className="text-xs text-muted">{`ends ${a.cycle.end}`}</span>}
              </div>
              <UsageBar name="5-hour" span={FIVE_HOURS} w={a.five_hour} />
              <UsageBar name="Week" span={WEEK} w={a.weekly} />
              <div className="flex gap-2 text-xs text-muted">
                <span>{`${a.agents.length} agents, up to ${a.cap_effective} at once`}</span>
                {a.agents.length > 0 && <span className="truncate text-fg">{a.agents.join(', ')}</span>}
              </div>
            </section>
          ))}
        </div>
      </Page>
    </ScrollFade>
  );
}
