import type { PendingRequest, RequestRow } from '@handline/client';
import { ago } from '#src/format/time.ts';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';

const ASK: Record<PendingRequest['kind'], string> = {
  allow: 'Allow?',
  decide: 'Decide',
  ask: 'Question',
  warning: 'Warning',
  comment: 'New comments',
};

function Line({ title, meta, onOpen, mark }: { title: string; meta: string; onOpen: (how: 'here' | 'side') => void; mark?: boolean }) {
  return (
    <button type="button" {...openProps(onOpen)} className="flex w-full gap-2.5 rounded-md px-2 py-1.5 text-left hover:bg-hover/30">
      <span className={`mt-1.5 size-1.5 shrink-0 rounded-full ${mark ? 'bg-accent' : ''}`} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-fg">{title}</span>
        <span className="truncate text-xs text-muted">{meta}</span>
      </span>
    </button>
  );
}

/** Something waiting on the founder: what is asked, its task, how long ago. A click opens the task, else Requests, where an item with no task is answered. */
export function TurnLine({ p, onOpened }: { p: PendingRequest; onOpened?: () => void }) {
  const openTab = useOpenTab();
  return (
    <Line
      mark
      title={p.title}
      meta={[ASK[p.kind], p.task !== null && `T${p.task}`, ago(p.last_at)].filter(Boolean).join(' · ')}
      onOpen={(how) => {
        openTab(p.task !== null ? taskOf(p.task) : 'requests', how);
        onOpened?.();
      }}
    />
  );
}

const STATUS: Record<RequestRow['status'], string> = {
  waiting_on_you: 'waiting on you',
  review: 'in review',
  doing: 'in progress',
  done: 'done',
  no_work: 'no work needed',
  not_yet: 'not started',
};

/** One of the founder's requests: its title, who works on it now (or how it ended), when it last moved. A click opens its task, else
 * the chat it was said in. */
export function RequestLine({ r, onOpened }: { r: RequestRow; onOpened?: () => void }) {
  const openTab = useOpenTab();
  const who = r.now ?? r.holders[0] ?? r.agent;
  const meta = r.status === 'done' || r.status === 'no_work' ? [STATUS[r.status], r.agent] : [who, r.progress || STATUS[r.status]];
  return (
    <Line
      title={r.title}
      meta={[...meta, ago(r.last_activity)].filter(Boolean).join(' · ')}
      onOpen={(how) => {
        openTab(r.task !== null ? taskOf(r.task) : chatOf(r.agent), how);
        onOpened?.();
      }}
    />
  );
}
