import { useWrite } from '@handline/app-data';
import type { PendingRequest, RequestRow } from '@handline/client';
import { Cpu } from 'lucide-react';
import type { ReactNode } from 'react';
import { ago } from '#src/format/time.ts';
import { focusIn } from '#src/layout/focus.ts';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';
import { openSettings } from '#src/settings/open.ts';

const ASK: Record<PendingRequest['kind'], string> = {
  allow: 'Allow?',
  decide: 'Decide',
  ask: 'Question',
  warning: 'Warning',
  comment: 'New comments',
};

function Line({
  title,
  meta,
  onOpen,
  mark,
  icon,
}: {
  title: string;
  meta: string;
  onOpen: (how: 'here' | 'side') => void;
  mark?: boolean;
  icon?: ReactNode;
}) {
  return (
    <button type="button" {...openProps(onOpen)} className="flex w-full gap-2.5 rounded-md px-2 py-1.5 text-left hover:bg-hover/30">
      <span className={`mt-1.5 size-1.5 shrink-0 rounded-full ${mark ? 'bg-accent' : ''}`} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-fg">{title}</span>
        <span className="flex items-center gap-1 truncate text-xs text-muted">
          {icon}
          {meta}
        </span>
      </span>
    </button>
  );
}

/** Where a row opens: its task; for a daemon warning, the account, the chat or the settings it is about; else Requests. */
function useOpenTurn(p: PendingRequest) {
  const openTab = useOpenTab();
  return (how: 'here' | 'side') => {
    const w = p.warning;
    if (p.task !== null) return openTab(taskOf(p.task), how);
    if (w?.account && (w.kind === 'login' || w.kind === 'weekly_use')) {
      focusIn('usage', '', w.account);
      return openTab('usage', how);
    }
    if (w?.agent) return openTab(chatOf(w.agent), how);
    if (w?.kind === 'disk') return openSettings();
    openTab('requests', how);
  };
}

/** Something waiting on the founder: what is asked, who asks (an agent, or the daemon itself), how long ago. */
export function TurnLine({ p, onOpened }: { p: PendingRequest; onOpened?: () => void }) {
  const open = useOpenTurn(p);
  const daemon = p.kind === 'warning' || p.agent === null;
  return (
    <Line
      mark
      title={p.title}
      icon={daemon && <Cpu size={11} aria-hidden className="shrink-0" />}
      meta={[ASK[p.kind], daemon ? 'daemon' : p.agent, p.task !== null && `T${p.task}`, p.ask, ago(p.last_at)].filter(Boolean).join(' · ')}
      onOpen={(how) => {
        open(how);
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

/** A finished request the founder has not opened since it ended. */
export const isUnseen = (r: RequestRow): boolean => r.ended?.seen === false;

/** One of the founder's requests: its title, who works on it now (or how it ended), when it last moved. A click opens its task, else
 * the chat it was said in; a result not seen yet is marked, and opening it marks it seen. */
export function RequestLine({ r, onOpened }: { r: RequestRow; onOpened?: () => void }) {
  const openTab = useOpenTab();
  const seen = useWrite('task.seen', r.task === null ? undefined : String(r.task));
  const who = r.now ?? r.holders[0] ?? r.agent;
  const ended = r.ended?.outcome === 'discarded' ? 'discarded' : STATUS[r.status];
  const meta = r.status === 'done' || r.status === 'no_work' ? [ended, r.agent] : [who, r.progress || STATUS[r.status]];
  return (
    <Line
      mark={isUnseen(r)}
      title={r.title}
      meta={[...meta, ago(r.ended?.at ?? r.last_activity)].filter(Boolean).join(' · ')}
      onOpen={(how) => {
        if (r.task !== null && isUnseen(r)) seen.mutate({ task: r.task });
        openTab(r.task !== null ? taskOf(r.task) : chatOf(r.agent), how);
        onOpened?.();
      }}
    />
  );
}
