// Notifications, opened from the status bar's bell: the inbox newest first, unread ones marked. A click opens what an item is about
// (its task, else its agent's chat) and marks it read; Mark all read clears the bell's dot.
import { hooks, useWrite } from '@handline/app-data';
import type { InboxItem } from '@handline/client';
import { ago } from '#src/format/time.ts';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

export function InboxTab() {
  const q = hooks.useInboxList({ limit: 100 });
  // Mark all read takes every unread item, not only those in the newest 100 shown, so the bell's dot clears.
  const unread = hooks.useInboxList({ unread: true, limit: 200 }).data?.items ?? [];
  const read = useWrite('inbox.mark_read');
  const items = q.data?.items ?? [];
  return (
    <ScrollFade keep="inbox" className="min-h-0 flex-1">
      <Page className="flex flex-col gap-3 py-4 text-sm">
        <header className="flex items-center gap-3">
          <h1 className="flex-1 text-lg font-semibold">Notifications</h1>
          {unread.length > 0 && (
            <Button variant="secondary" onClick={() => read.mutate({ ids: unread.map((i) => i.id) })}>
              Mark all read
            </Button>
          )}
        </header>
        {q.isError && <p className="text-danger">Notifications did not load.</p>}
        {q.data && !items.length && <p className="text-muted">Nothing yet.</p>}
        <div className="flex flex-col">
          {items.map((i, n) => (
            <Row key={i.id} item={i} ruled={n > 0} onRead={() => !i.read && read.mutate({ ids: [i.id] })} />
          ))}
        </div>
      </Page>
    </ScrollFade>
  );
}

function Row({ item, ruled, onRead }: { item: InboxItem; ruled: boolean; onRead: () => void }) {
  const openTab = useOpenTab();
  const target = item.task != null ? taskOf(item.task) : item.agent ? chatOf(item.agent) : null;
  return (
    <button
      type="button"
      {...openProps((how) => {
        onRead();
        if (target) openTab(target, how);
      })}
      className={`flex gap-2.5 px-3 py-2 text-left hover:bg-hover/30 ${ruled ? 'border-t border-border' : ''}`}
    >
      <span className={`mt-1.5 size-2 shrink-0 rounded-full ${item.read ? '' : 'bg-accent'}`} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className={`truncate ${item.read ? 'text-muted' : 'font-medium text-fg'}`}>{item.title}</span>
        <span className="truncate text-xs text-muted">
          {[item.agent, item.task != null && `T${item.task}`, ago(item.created)].filter(Boolean).join(' · ')}
        </span>
      </span>
    </button>
  );
}
